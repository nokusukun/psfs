package satellite

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/minio/sha256-simd"

	messages "github.com/nokusukun/psfs/pb"
)

func NewBroadcaster(parent *Satellite) *Broadcaster {
	br := Broadcaster{}
	br.Satellite = parent
	br.PacketBroadcasts = make(map[string][]string)

	return &br
}

type Broadcaster struct {
	Satellite        *Satellite
	PacketBroadcasts map[string][]string
}

func (b *Broadcaster) Broadcast(ssp *messages.SecureStreamPacket) error {
	for pid, peer := range b.Satellite.Peers {
		securePacketBytes, _ := proto.Marshal(ssp)
		err := peer.writeToStream(base64.StdEncoding.EncodeToString(securePacketBytes))
		if err != nil {
			logger.Error(pid, "failed to rebroadcast/write:", err)
		}
	}
	return nil
}

func (b *Broadcaster) Rebroadcast(packet *PeerMessage) {
	if packet.Source.ID == b.Satellite.ID {
		return
	}

	packetID := HashPacket(packet)
	logger.Info("Broadcasting", packetID)

	if PacketExpired(packet) {
		logger.Debug(packetID, "Packet is expired, disposing")
		// Todo: should be handled by the broadcasts GC?
		return
	}

	// Start broadcasting to all of the peers
	for pid, peer := range b.Satellite.Peers {
		broadcastedPeers, exists := b.PacketBroadcasts[packetID]

		// Create a broadcasted peers slice
		if !exists {
			broadcastedPeers = []string{}
			b.PacketBroadcasts[packetID] = broadcastedPeers
		}

		// Check if the peer has already been broadcasted to
		skip := false
		for _, bpid := range broadcastedPeers {
			if bpid == pid {
				skip = true
			}
		}

		if skip {
			continue
		}

		// Rebroadcast based on the suggested broadcast ratio
		if rand.Float64() <= b.Satellite.Config.BroadcastRatio {
			logger.Info("Broadcasting", packetID, "to", pid)
			securePacketBytes, _ := proto.Marshal(packet.SSP)
			err := peer.writeToStream(base64.StdEncoding.EncodeToString(securePacketBytes))
			if err != nil {
				logger.Error(pid, "failed to rebroadcast/write:", err)
			}
			broadcastedPeers = append(broadcastedPeers, pid)
		}
	}

}

func HashPacket(packet *PeerMessage) string {
	return HashPacketSSP(packet.SSP)
}

func HashPacketSSP(packet *messages.SecureStreamPacket) string {
	b, err := proto.Marshal(packet)
	if err != nil {
		panic(fmt.Errorf("failed to hash the packet: %v", err))
	}

	sha256encoder := sha256.New()
	sha256encoder.Write(b)
	// bs := hex.EncodeToString(sha256encoder.Sum([]byte(""))[:32])
	bs := base64.StdEncoding.EncodeToString(sha256encoder.Sum([]byte("")))
	return bs
}

func PacketExpired(packet *PeerMessage) bool {
	now := time.Now().Unix()
	freshUntil := packet.Packet.Lifespan + packet.Packet.Timestamp

	return freshUntil-now > 0
}
