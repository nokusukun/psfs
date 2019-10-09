## PSFS
*Pretty Shitty File System (This is not a filesystem)*

This project serves as a testbed and framework for a decentralized platform.

The important part is basically `/psfs/satellite`, this module handles all of the finding of peers and connection.

### 🌟Features!🌟
* Rendezvous DHT
* Automated Peer lifecycle handling
* Built in Peer Request/Response
* Broadcasting
* Satellite Events


### Events
PSFS has several events as defined in the `messages.proto`
document
```proto
enum PacketType {
    INTERNAL    = 0;
    MESSAGE     = 1;
    BROADCAST   = 2;
    SEEK        = 3;

    REQUEST     = 4;
    RESPONSE    = 5;
    END_RESPONSE = 6;
}
```
```
* Internal
    * Used by internal PSFS events
* Message
    * One way messages
* Broadcast
    * Self explanatory, gets sent to the other peers that are 
    connected to the satellite.
* Seek
    * Might be deprecated in favor of `Broadcast+Request`

* Request
    * Request packet in which a `Response` packet is expected.
* Response
    * A response packet, used to be in conjunction with the `Request packet`
* EndResponse
    * A special response packet which notifies the specific `responseID` that the request is finished and that the
    stream should be closed.
```

### Event callbacks
Event callbacks lets you handle whatever gets sent your way, think REST API.
All of the events except for Request and Broadcast+Request events doesn't need a response.
```go
satellite.Event(messages.PacketType_MESSAGE, "greeting", func(p *satellite.PeerMessage) error {
    fmt.Println("Recieved Message from:", satellite.PeerIDFy(p.Source.ID.String()), string(p.Packet.Content))
    return nil
})
```
#### Request Events
Reqeust events are special events where you can respond to the requesting message
```go
satellite.Event(messages.PacketType_REQUEST, "whois", func(p *satellite.PeerMessage) error {
    logger.Infof("Recieved whois request")
    err := p.Reply(satellite.J{"id": sat.Host.ID().Pretty(), "b2048": b2048.Encode([]byte(sat.Host.ID().Pretty()))})
    if err != nil {
        return err
    }
    return nil
})
```

### Security
Satellites are inherently secure, connecting requires a 2048bit RSA key in order to interact with each other.
Each packet is signed, but PSFS features a `lazysec` mode where the peers only need to sign the first packet to assume
an authenticated status. Future packets aren't signed afterwards.

Though, there are still several flaws that are still around.