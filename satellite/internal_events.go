package satellite

import (
	messages "github.com/nokusukun/psfs/pb"
)

func bindInternalEvents(sat *Satellite) {
	sat.Event(messages.PacketType_INTERNAL, "__internal_ping", func(message *PeerMessage) error {
		_ = message.Source.Write(messages.PacketType_INTERNAL, "__psfs_pong", message.Packet.Content)
		return nil
	})

	sat.Event(messages.PacketType_INTERNAL, "__allow_lazy_secure", func(message *PeerMessage) error {
		logger.Info(PeerIDFy(message.Source.ID.String()), "has allowed lazy security")
		message.Source.AllowLazySecure = true
		return nil
	})

}
