package hybrid2

import "time"

const (
	// RepliableInterval defines how often to send a datagram2 for identity refresh.
	// Pattern: 1 datagram2 (at index 0), then 99 datagram3, then 1 datagram2 (at index 100), repeat.
	// This means every 100th message is authenticated via datagram2.
	RepliableInterval = 100

	// FirstMessageIndex is the counter value for the first datagram2 (identity establishment).
	// When counter % RepliableInterval == FirstMessageIndex, datagram2 is used.
	FirstMessageIndex = 0

	// HashExpiryDuration is how long to keep a sender hash mapping before expiry.
	// After this duration without a refresh (new datagram2), the mapping is considered stale.
	// This provides a balance between memory efficiency and allowing for network delays.
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
)
