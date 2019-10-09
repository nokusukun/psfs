package satellite

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	messages "github.com/nokusukun/psfs/pb"
)

var (
	MAX_RESPONSE_STREAM_LIFETIME = time.Second * 30
)

type PeerMessage struct {
	Packet *messages.StreamPacket
	Source *Peer
	SSP    *messages.SecureStreamPacket
}

func (pm *PeerMessage) Reply(data interface{}) error {
	responseId := HashPacket(pm)
	logger.Infof("Responding to %v", responseId)
	logger.Debug("CONTENT: ", data)
	return pm.Source.Write(messages.PacketType_RESPONSE, responseId, data)
}

func (pm *PeerMessage) EndReply() error {
	responseId := HashPacket(pm)
	logger.Infof("Finishing response to %v", responseId)
	return pm.Source.Write(messages.PacketType_END_RESPONSE, responseId, nil)
}

type Peer struct {
	Stream       network.Stream
	Connection   network.Conn
	ID           peer.ID
	OnDisconnect func(*Peer)
	Sat          *Satellite

	wrLock          *sync.RWMutex
	wr              *bufio.ReadWriter
	Authenticated   bool
	AllowLazySecure bool
	Closed          bool
}

func (p *Peer) Request(ns string, data interface{}) (chan interface{}, error) {
	contentBytes, err := json.Marshal(data)

	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: ", err)
	}

	timestamp := time.Now().Unix()

	securePacket := p.Sat.SignPacket(&messages.StreamPacket{
		Type:      messages.PacketType_REQUEST,
		Namespace: ns,
		Content:   contentBytes,
		Timestamp: timestamp,
		Lifespan:  0,
	}, p.AllowLazySecure)

	// Marshal the data into a stream friendly format
	// Proto -> Bytes -> base64
	securePacketBytes, err := proto.Marshal(securePacket)

	// TODO: Add multi response support
	requestID := HashPacketSSP(securePacket)
	responseStream := make(chan interface{}, 100)
	responseStreamClosed := false

	// Listen for a response event
	p.Sat.Event(messages.PacketType_RESPONSE, requestID, func(pm *PeerMessage) error {
		// Check if the response peer is the same as the request peer
		if pm.Source.ID != p.ID {
			err := fmt.Errorf(`"%v" attempted to respond to request ID "%v" meant for "%v"`,
				pm.Source.ID,
				requestID,
				p.ID,
			)
			logger.Warning(err)
			return err
		}

		var cont interface{}
		err = json.Unmarshal(pm.Packet.Content, &cont)
		if err != nil {
			logger.Error("failed to unmarshal Response:", err)
			return err
		}

		responseStream <- cont
		return nil
	})

	p.Sat.Event(messages.PacketType_END_RESPONSE, requestID, func(pm *PeerMessage) error {
		// Check if the response peer is the same as the request peer
		if pm.Source.ID != p.ID {
			err := fmt.Errorf(`"%v" attempted to respond to request ID "%v" meant for "%v"`,
				pm.Source.ID,
				requestID,
				p.ID,
			)
			logger.Warning(err)
			return err
		}
		if !responseStreamClosed {
			close(responseStream)
			responseStreamClosed = true
		}
		return nil
	})

	// Writes the request packet to the destination peer
	logger.Infof("Sending Request %v to %v as %v", ns, p.ID.Pretty(), requestID)
	err = p.writeToStream(base64.StdEncoding.EncodeToString(securePacketBytes))
	if err != nil {
		close(responseStream)
		return nil, err
	}

	// Ensure to close down the response stream after a specified amount of time
	go func() {
		time.Sleep(MAX_RESPONSE_STREAM_LIFETIME)
		if !responseStreamClosed {
			close(responseStream)
			responseStreamClosed = true
		}
	}()

	return responseStream, nil
}

func (p *Peer) Disconnect() error {
	p.Closed = true
	logger.Info("Disconnecting: ", p.ID)
	p.OnDisconnect(p)
	return p.Stream.Close()
}

func (p *Peer) assemblePeerMessage(message []byte) (*PeerMessage, error) {
	sPacket := &messages.SecureStreamPacket{}
	err := proto.Unmarshal(message, sPacket)

	if err != nil {
		return nil, err
	}

	if IsExpired(sPacket) {
		return nil, fmt.Errorf("packet is expired")
	}

	if !p.Authenticated || !p.Sat.Config.LazySecure {
		if validated, err := ValidateSSP(sPacket); err != nil || !validated {
			return nil, fmt.Errorf("packet validation failed: %v", err)
		}

		p.Authenticated = true

		if p.Sat.Config.LazySecure {
			err := p.Write(messages.PacketType_INTERNAL, "__allow_lazy_secure", true)
			if err != nil {
				logger.Error("failed to allow lazy security:", err)
			}
		}
	}

	pm := &PeerMessage{
		sPacket.Packet,
		p,
		sPacket,
	}

	return pm, nil
}

func (p *Peer) Bootstrap(stream network.Stream, inbound chan *PeerMessage) {
	p.Stream = stream
	p.ID = stream.Conn().RemotePeer()
	p.Connection = stream.Conn()

	fmt.Println("Bootstrapping", PeerIDFy(p.ID.String()))
	logger.Info("Bootstrapping: ", PeerIDFy(p.ID.String()))
	p.wr = bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

	go func() {
		for {
			str, err := p.wr.ReadString('\n')
			if err != nil {
				logger.Warning("Error reading from buffer, disconnecting:", err)
				_ = p.Disconnect()
				return
			}

			packetBytes, err := base64.StdEncoding.DecodeString(str)

			if string(packetBytes) != "\n" {
				pm, err := p.assemblePeerMessage(packetBytes)
				if err != nil {
					logger.Error("Failed to parse packet:", err)
				} else {
					inbound <- pm
				}
			}

		}
	}()
}

// ONLY WRITE BASE64 ENCODED SECURESTREAMPACKETS
func (p *Peer) writeToStream(packetStr string) error {
	p.wrLock.Lock()
	defer p.wrLock.Unlock()

	_, err := p.wr.WriteString(fmt.Sprintf("%v\n", packetStr))
	if err != nil {
		_ = p.wr.Flush()
		return fmt.Errorf("failed to write to peer: ", err)
	}

	err = p.wr.Flush()
	if err != nil {
		return fmt.Errorf("failed to flush data", err)
	}

	return nil
}

func (p *Peer) Write(t messages.PacketType, ns string, data interface{}) error {
	contentBytes, err := json.Marshal(data)

	if err != nil {
		return fmt.Errorf("failed to marshal data: ", err)
	}

	timestamp := time.Now().Unix()

	securePacket := p.Sat.SignPacket(&messages.StreamPacket{
		Type:      t,
		Namespace: ns,
		Content:   contentBytes,
		Timestamp: timestamp,
		Lifespan:  0,
	}, p.AllowLazySecure)

	// Marshal the data into a stream friendly format
	// Proto -> Bytes -> base64
	securePacketBytes, err := proto.Marshal(securePacket)

	return p.writeToStream(base64.StdEncoding.EncodeToString(securePacketBytes))
}
