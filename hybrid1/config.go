package hybrid1

import "time"

// Mode constants for selecting hybrid protocol version.
const (
	// HYBRID_MODE_V1 selects i2pd-compatible datagram protocol.
	// Use this mode for interoperability with i2pd-based applications.
	HYBRID_MODE_V1 = 1

	// HYBRID_MODE_V2 selects Go-native SAM datagram protocol (default).
	// This is the recommended mode for Go-to-Go communication.
	HYBRID_MODE_V2 = 2
)

// I2P datagram constants.
const (
	// I2P_UDP_MAX_MTU is the maximum MTU for I2P UDP datagrams (64KB).
	I2P_UDP_MAX_MTU = 64 * 1024

	// I2P_UDP_REPLIABLE_DATAGRAM_INTERVAL is the interval for sending repliable datagrams.
	// This ensures session state is maintained on long-lived connections.
	I2P_UDP_REPLIABLE_DATAGRAM_INTERVAL = 100 * time.Millisecond

	// DefaultBufferSize is the default buffer size for datagram operations.
	DefaultBufferSize = I2P_UDP_MAX_MTU

	// DefaultReceiveChannelSize is the buffer size for the receive channel.
	DefaultReceiveChannelSize = 100
)

// i2pd protocol constants.
const (
	// i2pd datagram header structure:
	// - 2 bytes: source port (big-endian)
	// - 2 bytes: destination port (big-endian)
	// - payload follows

	// I2PD_HEADER_SIZE is the size of the i2pd datagram header.
	I2PD_HEADER_SIZE = 4

	// I2PD_PORT_SIZE is the size of each port field in the header.
	I2PD_PORT_SIZE = 2

	// I2PD_DEFAULT_PORT is the default port when none is specified.
	I2PD_DEFAULT_PORT = 0

	// I2PD_MAX_PAYLOAD_SIZE is the maximum payload size after header overhead.
	I2PD_MAX_PAYLOAD_SIZE = I2P_UDP_MAX_MTU - I2PD_HEADER_SIZE
)

// Session configuration options.
var (
	// RepliableInterval defines how often to send a repliable datagram for identity refresh.
	// Every Nth message is sent as a repliable datagram.
	RepliableInterval = 100

	// HashExpiryDuration is how long to keep a sender hash mapping before expiry.
	HashExpiryDuration = 10 * time.Minute

	// DefaultReadTimeout is the default timeout for read operations.
	DefaultReadTimeout = 30 * time.Second

	// DefaultWriteTimeout is the default timeout for write operations.
	DefaultWriteTimeout = 10 * time.Second
)
