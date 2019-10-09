package satellite

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	mrand "math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	discovery "github.com/libp2p/go-libp2p-discovery"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/mr-tron/base58"
	"github.com/multiformats/go-multiaddr"
	"github.com/whyrusleeping/go-logging"

	"github.com/nokusukun/psfs/configuration"
	messages "github.com/nokusukun/psfs/pb"
)

var logger = log.Logger("satellite")

type Satellite struct {
	ID              peer.ID
	Peers           map[string]*Peer
	MessageBindings map[string]func(*PeerMessage) error
	OnPeerConnect   func(*Peer)
	Inbound         chan *PeerMessage
	peerLock        *sync.RWMutex
	DHT             *dht.IpfsDHT
	Host            host.Host
	Config          configuration.Config
	Broadcaster     *Broadcaster

	PubKey  crypto.PubKey
	PrivKey crypto.PrivKey
}

func Create(config configuration.Config) *Satellite {

	satellite := &Satellite{}
	satellite.Peers = map[string]*Peer{}
	satellite.MessageBindings = map[string]func(*PeerMessage) error{}
	satellite.Inbound = make(chan *PeerMessage, 500)
	satellite.peerLock = &sync.RWMutex{}
	satellite.OnPeerConnect = func(p *Peer) {}
	satellite.Config = config
	satellite.Broadcaster = NewBroadcaster(satellite)

	_ = log.SetLogLevel("satellite", logging.INFO.String())

	ctx := context.Background()

	var r io.Reader
	if config.Debug {
		// Use the port number as the randomness source.
		// This will always generate the same satHost ID on multiple executions, if the same port number is used.
		// Never do this in production code.
		hdx := sha1.New()
		hdx.Write([]byte(config.ListenAddresses.String()))
		source, err := strconv.ParseInt(fmt.Sprintf("%x", hdx.Sum(nil)[:5]), 16, 64)
		if err != nil {
			panic(err)
		}
		r = mrand.New(mrand.NewSource(source))
	} else {
		r = rand.Reader
	}

	logger.Info("Generating RSA Keys")
	prvKey, pubKey, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	if err != nil {
		panic(err)
	}

	satellite.PrivKey = prvKey
	satellite.PubKey = pubKey

	pkb, _ := pubKey.Bytes()
	logger.Info(base58.Encode(pkb))

	// libp2p.New constructs a new libp2p Host. Other options can be added
	// here.
	satHost, err := libp2p.New(ctx,
		libp2p.ListenAddrs([]multiaddr.Multiaddr(config.ListenAddresses)...),
		libp2p.Identity(prvKey),
	)

	satellite.ID = satHost.ID()

	if err != nil {
		panic(err)
	}
	logger.Info("Host created. We are:", PeerIDFy(satHost.ID().String()))
	logger.Info(satHost.Addrs())

	satellite.Host = satHost

	// Set a function as stream handler. This function is called when a peer
	// initiates a connection and starts a stream with this peer.
	satHost.SetStreamHandler(protocol.ID(config.ProtocolID), satellite.HandlePeer)

	bindInternalEvents(satellite)

	return satellite
}

func (s *Satellite) Connect() {

	go s.inboundProcessor()

	ctx := context.Background()
	satHost := s.Host
	config := s.Config

	// Start a DHT, for use in peer discovery. We can't just make a new DHT
	// client because we want each peer to maintain its own local copy of the
	// DHT, so that the bootstrapping node of the DHT can go down without
	// inhibiting future peer discovery.
	kademliaDHT, err := dht.New(ctx, satHost)
	if err != nil {
		panic(err)
	}

	// Bootstrap the DHT. In the default configuration, this spawns a Background
	// thread that will refresh the peer table every five minutes.
	logger.Debug("Bootstrapping the DHT")
	if err = kademliaDHT.Bootstrap(ctx); err != nil {
		panic(err)
	}

	// Let's connect to the bootstrap nodes first. They will tell us about the
	// other nodes in the network.
	var wg sync.WaitGroup
	for _, peerAddr := range config.BootstrapPeers {
		peerinfo, err := peer.AddrInfoFromP2pAddr(peerAddr)
		if err != nil {
			logger.Error(err, peerAddr)
		}
		logger.Info(peerinfo)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := satHost.Connect(ctx, *peerinfo); err != nil {
				logger.Error(err)
			} else {
				logger.Info("Connection established with bootstrap node:", *peerinfo)
			}
		}()
	}
	wg.Wait()

	logger.Info("Peer ID: ", PeerIDFy(kademliaDHT.PeerID().String()))
	s.DHT = kademliaDHT
	// We use a rendezvous point "meet me here" to announce our location.
	// This is like telling your friends to meet you at the Eiffel Tower.
	logger.Info("Announcing ourselves...")
	routingDiscovery := discovery.NewRoutingDiscovery(kademliaDHT)
	discovery.Advertise(ctx, routingDiscovery, config.RendezvousString)
	logger.Info("Successfully announced!")

	// Endlessly loop finding peers
	go func() {
		lastRun := time.Now()
		for {
			s.findPeers(ctx, routingDiscovery, satHost)
			// Check if the last run has been less than 5 minutes
			if time.Now().Sub(lastRun) < time.Minute*5 {
				sleepDur := time.Now().Sub(lastRun)
				logger.Info("Sleeping findpeer routine for: ", sleepDur)
				time.Sleep(sleepDur)
			}
		}
	}()

}

func (s *Satellite) findPeers(ctx context.Context, routingDiscovery *discovery.RoutingDiscovery, satHost host.Host) {
	config := s.Config

	tctx, cancel := context.WithTimeout(ctx, time.Minute*30)
	defer func() {
		cancel()
		logger.Info("Finished find connection cycle")
	}()

	// Now, look for others who have announced
	// This is like your friend telling you the location to meet you.
	logger.Info("Searching for other peers...")
	peerConnectionStream, err := routingDiscovery.FindPeers(tctx, config.RendezvousString)
	if err != nil {
		panic(err)
	}

	for connection := range peerConnectionStream {
		if connection.ID == satHost.ID() {
			continue
		}

		logger.Debug("Found connection:", PeerIDFy(connection.ID.String()))

		logger.Debug("Connecting to:", PeerIDFy(connection.ID.String()))
		stream, err := satHost.NewStream(ctx, connection.ID, protocol.ID(config.ProtocolID))

		if err != nil {
			logger.Warning("Connection failed:", err)
			continue
		} else {
			s.HandlePeer(stream)
		}

		logger.Info("Connected to:", PeerIDFy(connection.ID.String()))
	}
}

// SignPacket signs a StreamPacket and returns a SignedStreamPacket, passing true
// skips any signing and just encapsulates it in a SecureStreamPacket
func (s *Satellite) SignPacket(packet *messages.StreamPacket, skipSigning bool) *messages.SecureStreamPacket {
	var err error

	var sig []byte
	var src []byte

	// Lazy security has been enabled, don't bother signing the packet
	// Though always sign broadcast packets
	if !skipSigning || packet.Type == messages.PacketType_BROADCAST {
		var packetBytes []byte
		if packetBytes, err = proto.Marshal(packet); err != nil {
			panic(err)
		}

		// Sign packet bytes
		if sig, err = s.PrivKey.Sign(packetBytes); err != nil {
			panic(err)
		}

		// Append signer public key
		if src, err = s.PubKey.Raw(); err != nil {
			panic(err)
		}
	}

	return &messages.SecureStreamPacket{
		Packet:    packet,
		Signature: sig,
		Source:    src,
	}
}

// BroadcastMessage broadcasts a message in a specific namespace
func (s *Satellite) BroadcastMessage(namespace string, data interface{}) error {
	contentBytes, err := json.Marshal(data)

	if err != nil {
		return fmt.Errorf("failed to marshal data: ", err)
	}

	timestamp := time.Now().Unix()

	packet := &messages.StreamPacket{
		Type:      messages.PacketType_BROADCAST,
		Namespace: namespace,
		Content:   contentBytes,
		Timestamp: timestamp,
		Lifespan:  s.Config.BroadcastLifetime,
	}

	ssp := s.SignPacket(packet, false)
	return s.Broadcaster.Broadcast(ssp)
}

// Inbound Processor is a gorouting that loops through the Inbound PeerMessages and passes
// it through any events that exists
func (s *Satellite) inboundProcessor() {
	logger.Info("Starting Inbound Processor")
	for message := range s.Inbound {
		// Rebroadcast if the message is a Broadcast
		if message.Packet.Type == messages.PacketType_BROADCAST {
			s.Broadcaster.Rebroadcast(message)
		}

		logger.Infof("Received Inbound: %v %v", message.Packet.Type.String(), message.Packet.Namespace)

		f, exists := s.MessageBindings[message.Packet.Type.String()+message.Packet.Namespace]
		if exists {
			err := f(message)
			// Return a reply is something failed on a Request message
			if err != nil && message.Packet.Type == messages.PacketType_REQUEST {
				_ = message.Reply(J{"__INTERNAL_ERROR": fmt.Sprint(err), "__SOURCE": "RESPONSE"})
			}
			if message.Packet.Type == messages.PacketType_REQUEST {
				_ = message.EndReply()
			}
		}
	}
}

func (s *Satellite) Event(eventType messages.PacketType, namespace string, onEvent func(*PeerMessage) error) {
	s.MessageBindings[eventType.String()+namespace] = onEvent
}

// HandlePeer gets executed everytime a new peer gets discovered in the DHT, accepts a network stream
// and stores it in Satellite.Peers mapping
func (s *Satellite) HandlePeer(stream network.Stream) {

	if oldPeer, exists := s.Peers[stream.Conn().RemotePeer().String()]; exists && !oldPeer.Closed {
		return
	}

	// Create a buffer stream for non blocking read and write.
	newPeer := &Peer{}
	newPeer.Sat = s
	newPeer.Bootstrap(stream, s.Inbound)
	newPeer.wrLock = &sync.RWMutex{}

	newPeer.OnDisconnect = func(p *Peer) {
		s.peerLock.Lock()
		defer s.peerLock.Unlock()
		delete(s.Peers, p.ID.String())
	}

	s.peerLock.Lock()
	defer s.peerLock.Unlock()
	s.Peers[newPeer.Connection.RemotePeer().String()] = newPeer

	go s.OnPeerConnect(newPeer)
}
