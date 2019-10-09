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