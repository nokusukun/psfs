package satellite

import (
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/mr-tron/base58"
	"github.com/nokusukun/b2048"
	_ "github.com/nokusukun/stemp"

	"github.com/libp2p/go-libp2p-core/crypto"

	messages "github.com/nokusukun/psfs/pb"
)

type J map[string]interface{}

func PeerIDFy(peerid string) string {
	decoded, _ := base58.Decode(peerid)

	suffix := b2048.Encode(decoded[29:])
	return peerid + ":" + suffix
}

func IsExpired(ssp *messages.SecureStreamPacket) bool {
	now := time.Now().Unix()
	//todo: put this in the config
	var maximumOffset int64 = 10 // seconds

	return (now - ssp.Packet.Timestamp) > maximumOffset

}

func IsSignedSSP(ssp *messages.SecureStreamPacket) bool {
	return len(ssp.Signature) != 0
}

func ValidateSSP(ssp *messages.SecureStreamPacket) (bool, error) {
	if !IsSignedSSP(ssp) {
		return false, fmt.Errorf("stream packet is unsigned")
	}

	//fmt.Println(stemp.CompileStruct("Packet: {Packet} Source: {Source} Signature: {Signature}", ssp))

	pk, err := crypto.PubKeyUnmarshallers[crypto.RSA](ssp.Source)
	if err != nil {
		return false, fmt.Errorf("failed to unmarshal source: %v", err)
	}

	payload, _ := proto.Marshal(ssp.Packet)
	return pk.Verify(payload, ssp.Signature)
}
