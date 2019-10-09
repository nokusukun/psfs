package configuration

import (
	"strings"

	maddr "github.com/multiformats/go-multiaddr"
)

type addrList []maddr.Multiaddr

func (al *addrList) String() string {
	strs := make([]string, len(*al))
	for i, addr := range *al {
		strs[i] = addr.String()
	}
	return strings.Join(strs, ",")
}

func (al *addrList) Set(value string) error {
	addr, err := maddr.NewMultiaddr(value)
	if err != nil {
		return err
	}
	*al = append(*al, addr)
	return nil
}

type Config struct {
	RendezvousString string
	BootstrapPeers   addrList
	ListenAddresses  addrList
	ProtocolID       string
	ConnectToPeer    string
	Debug            bool
	APIListener      string

	BroadcastRatio    float64
	MaxRebroadcasts   int64
	BroadcastLifetime int64
	// LazySecure skips the validation and signing of packets in an Authenticated peer
	LazySecure bool
}
