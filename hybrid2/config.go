package hybrid2

import "time"

var (
	// RepliableInterval defines how often to send a datagram2 for identity refresh.
	// Pattern: 1 datagram2 (at index 0), then 499 datagram3, then 1 datagram2 (at index 500), repeat.
	// This means every 500th message is authenticated via datagram2.
	RepliableInterval = 500

	// FirstMessageIndex is the counter value for the first datagram2 (identity establishment).
	// When counter % RepliableInterval == FirstMessageIndex, datagram2 is used.
	FirstMessageIndex = 0

	// RepliableTimeInterval is the maximum time between datagram2 refreshes.
	// If this duration elapses since the last datagram2, the next message will use datagram2
	// regardless of the counter value. This prevents hash mapping expiry on low-traffic
	// connections while maintaining efficiency on high-traffic paths.
	// Set to 4 minutes to provide 2.5× safety margin before HashExpiryDuration (10 minutes).
	RepliableTimeInterval = 4 * time.Minute

	// HashExpiryDuration is how long to keep a sender hash mapping before expiry.
	// After this duration without a refresh (new datagram2), the mapping is considered stale.
	// This provides a balance between memory efficiency and allowing for network delays.
	// Set to 10 minutes - with RepliableTimeInterval=4min, mappings are refreshed 2.5× faster.
	HashExpiryDuration = 10 * time.Minute

	// DefaultReceiveBufferSize is the buffer size for the receive channel.
	// This allows some buffering of incoming datagrams before blocking.
	DefaultReceiveBufferSize = 100

	// RecvChanBufferSize is the buffer size for the unified receive channel.
	// This is an alias for DefaultReceiveBufferSize used by the receive logic.
	RecvChanBufferSize = DefaultReceiveBufferSize

	// DefaultErrorBufferSize is the buffer size for the error channel.
	// This allows error reporting without blocking the receive goroutines.
	DefaultErrorBufferSize = 10

	// AckTimeout is how long to wait for an ACK before considering it lost.
	// Set to 10 seconds to accommodate high-latency I2P paths while detecting
	// failures reasonably quickly. Should be >> typical I2P RTT (1-3 seconds).
	AckTimeout = 10 * time.Second

	// MaxUnackedDatagrams is the maximum number of unacknowledged datagram2 messages
	// before considering the path degraded. Not a hard flow control limit - used as
	// a quality signal. When this limit is approached, path quality degrades.
	MaxUnackedDatagrams = 3

	// AckRequestInterval defines how often to request ACKs on datagram2 sends.
	// Value of 1 means every datagram2 requests an ACK (maximum monitoring).
	// Higher values reduce ACK overhead but decrease path quality visibility.
	AckRequestInterval = 1

	// AckCleanupInterval is how often to scan for and clean up stale unacked messages.
	// Set to half of AckTimeout to ensure timely cleanup without excessive CPU usage.
	AckCleanupInterval = AckTimeout / 2
)
