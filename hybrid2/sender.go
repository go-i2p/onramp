package hybrid2

import (
	"fmt"
	"time"

	"github.com/go-i2p/i2pkeys"
)

// getSenderState retrieves or creates the sender state for a specific destination.
// The state tracks how many datagrams have been sent to this destination to determine
// when to use datagram2 (authenticated) vs datagram3 (low-overhead).
func (h *HybridSession) getSenderState(dest i2pkeys.I2PAddr) *SenderState {
	key := dest.Base64()

	// Fast path: check if state already exists
	h.senderMu.RLock()
	state, exists := h.senderStates[key]
	h.senderMu.RUnlock()

	if exists {
		return state
	}

	// Slow path: create new state
	h.senderMu.Lock()
	defer h.senderMu.Unlock()

	// Double-check after acquiring write lock to avoid race condition
	if state, exists = h.senderStates[key]; exists {
		return state
	}

	state = &SenderState{
		Destination: dest,
		Counter:     0,
	}
	h.senderStates[key] = state
	return state
}

// shouldUseDatagram2 determines if the next message should use datagram2.
// Returns true for the first message (counter=0) and every RepliableInterval-th message.
func (s *SenderState) shouldUseDatagram2() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.Counter%RepliableInterval == FirstMessageIndex
}

// incrementCounter increments the send counter and updates the last send time.
// Returns the new counter value after incrementing.
func (s *SenderState) incrementCounter() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Counter++
	s.LastSendTime = time.Now()
	return s.Counter
}

// resetCounter resets the counter to a specific value.
// Used after forced datagram2 sends to reset the cycle.
func (s *SenderState) resetCounter(value uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Counter = value
	s.LastSendTime = time.Now()
}

// SendDatagram sends data to the specified destination using the hybrid protocol.
// It automatically chooses between datagram2 and datagram3 based on the send counter:
//   - Every 100th message (counter % 100 == 0) uses datagram2 for authentication
//   - All other messages use datagram3 for lower overhead
//
// This approach provides sender identification through periodic authentication
// while minimizing overhead for bulk traffic.
func (h *HybridSession) SendDatagram(data []byte, dest i2pkeys.I2PAddr) error {
	// Check if session is closed
	h.closeMu.RLock()
	if h.closed {
		h.closeMu.RUnlock()
		return ErrSessionClosed
	}
	h.closeMu.RUnlock()

	state := h.getSenderState(dest)

	if state.shouldUseDatagram2() {
		// Send authenticated datagram2 for identity establishment/refresh
		// DatagramSubSession embeds DatagramSession which has SendDatagram
		if err := h.datagram2Sub.SendDatagram(data, dest); err != nil {
			return fmt.Errorf("hybrid2 datagram2 send: %w", err)
		}
	} else {
		// Send low-overhead datagram3 for bulk traffic
		// Datagram3SubSession requires using NewWriter().SendDatagram()
		if err := h.datagram3Sub.NewWriter().SendDatagram(data, dest); err != nil {
			return fmt.Errorf("hybrid2 datagram3 send: %w", err)
		}
	}

	state.incrementCounter()
	return nil
}

// ForceDatagram2 forces a datagram2 send regardless of the counter value.
// This is useful when immediate identity establishment is needed, such as:
//   - Starting a new conversation
//   - Recovering from communication failures
//   - Ensuring the receiver has an up-to-date mapping
//
// After sending, the counter is reset to 1 (as if message 0 was just sent).
func (h *HybridSession) ForceDatagram2(data []byte, dest i2pkeys.I2PAddr) error {
	// Check if session is closed
	h.closeMu.RLock()
	if h.closed {
		h.closeMu.RUnlock()
		return ErrSessionClosed
	}
	h.closeMu.RUnlock()

	state := h.getSenderState(dest)

	// Send authenticated datagram2
	if err := h.datagram2Sub.SendDatagram(data, dest); err != nil {
		return fmt.Errorf("hybrid2 forced datagram2 send: %w", err)
	}

	// Reset counter to 1 (just sent message 0, next will be 1)
	state.resetCounter(1)

	return nil
}

// GetSenderCounter returns the current send counter for a destination.
// This is useful for debugging and monitoring the send pattern.
// Returns 0 if no state exists for the destination.
func (h *HybridSession) GetSenderCounter(dest i2pkeys.I2PAddr) uint64 {
	key := dest.Base64()

	h.senderMu.RLock()
	defer h.senderMu.RUnlock()

	if state, exists := h.senderStates[key]; exists {
		state.mu.Lock()
		defer state.mu.Unlock()
		return state.Counter
	}
	return 0
}

// SendDatagramTimeout sends data to the specified destination with a timeout.
// If the timeout is zero or negative, it behaves like SendDatagram (no timeout).
// Returns ErrTimeout if the operation exceeds the specified timeout duration.
func (h *HybridSession) SendDatagramTimeout(data []byte, dest i2pkeys.I2PAddr, timeout time.Duration) error {
	// If no timeout specified, use regular send
	if timeout <= 0 {
		return h.SendDatagram(data, dest)
	}

	// Use a channel to receive the result from the send goroutine
	done := make(chan error, 1)

	go func() {
		done <- h.SendDatagram(data, dest)
	}()

	// Wait for either completion or timeout
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return ErrTimeout
	}
}
