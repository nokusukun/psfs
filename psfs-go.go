package main

import (
	"fmt"
	"net/http"

	"github.com/ipfs/go-log"
	"github.com/nokusukun/b2048"
	"github.com/whyrusleeping/go-logging"

	messages "github.com/nokusukun/psfs/pb"
	"github.com/nokusukun/psfs/satellite"
)

var logger = log.Logger("PSFS")

func main() {
	_ = log.SetLogLevel("PSFS", logging.INFO.String())
	logger.Info("Starting PSFS")
	config, _ := ParseFlags()
	sat := satellite.Create(config)

	sat.OnPeerConnect = func(peer *satellite.Peer) {
		fmt.Println("Peer connected:", satellite.PeerIDFy(peer.ID.String()))
		err := peer.Write(messages.PacketType_MESSAGE, "greeting", "Hello World!")
		if err != nil {
			fmt.Println("Failed to write:", err)
		}
	}

	sat.Event(messages.PacketType_MESSAGE, "greeting", func(p *satellite.PeerMessage) error {
		fmt.Println("Recieved Message from:", satellite.PeerIDFy(p.Source.ID.String()), string(p.Packet.Content))
		return nil
	})

	sat.Event(messages.PacketType_BROADCAST, "seek", func(p *satellite.PeerMessage) error {
		fmt.Println(satellite.PeerIDFy(p.Source.ID.String()), "is seeking file", string(p.Packet.Content))
		return nil
	})

	sat.Event(messages.PacketType_REQUEST, "whois", func(p *satellite.PeerMessage) error {
		logger.Infof("Recieved whois request")
		err := p.Reply(satellite.J{"id": sat.Host.ID().Pretty(), "b2048": b2048.Encode([]byte(sat.Host.ID().Pretty()))})
		//err := p.Reply(b2048.Encode([]byte(sat.Host.ID().Pretty())))
		if err != nil {
			return err
		}
		return nil
	})

	go sat.Connect()

	router := generateAPI(sat)
	fmt.Println("Starting API")
	logger.Info("Starting API on:", config.APIListener)
	logger.Fatal(http.ListenAndServe(config.APIListener, router))

}
