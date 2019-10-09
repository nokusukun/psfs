package main

import (
	"flag"

	dht "github.com/libp2p/go-libp2p-kad-dht"

	"github.com/nokusukun/psfs/configuration"
)

func ParseFlags() (configuration.Config, error) {
	config := configuration.Config{}
	flag.StringVar(&config.RendezvousString, "r", "pineapple13",
		"Unique string to identify group of nodes. Share this with your friends to let them connect with you")
	flag.Var(&config.BootstrapPeers, "peer", "Adds a peer multiaddress to the bootstrap list")
	flag.Var(&config.ListenAddresses, "listen", "Adds a multiaddress to the listen list")
	flag.StringVar(&config.ProtocolID, "pid", "/chat/1.1.0", "Sets a protocol id for stream headers")
	flag.StringVar(&config.ConnectToPeer, "findpeer", "", "")
	flag.BoolVar(&config.Debug, "debug", false, "Run the client in debug mode")
	flag.StringVar(&config.APIListener, "api", ":3001", "Bind api listener")
	flag.BoolVar(&config.LazySecure, "lazysec", false, "Utilize lazy security")

	flag.Float64Var(&config.BroadcastRatio, "broadcastratio", 1, "Suggested ratio to broadcast")
	flag.Int64Var(&config.MaxRebroadcasts, "maxrebroadcasts", 5, "Maximum rebroadcasts for a given packet")
	flag.Int64Var(&config.BroadcastLifetime, "broadcastlifetime", 30, "Broadcast packet lifetime")

	flag.Parse()

	if len(config.BootstrapPeers) == 0 {
		config.BootstrapPeers = dht.DefaultBootstrapPeers
	}

	return config, nil
}
