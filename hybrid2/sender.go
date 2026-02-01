package hybrid2

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/go-i2p/i2pkeys"
)

// ACK message format markers
const (
	// AckRequestMarker is prepended to datagram2 payload when ACK is requested
	AckRequestMarker = byte(0xAC) // 'AC'k request

	// AckResponseMarker is used for ACK response messages
	AckResponseMarker = byte(0xAD) // 'AD'ack

	// Regular data messages have no marker (backward compatible)
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
		Destination:      dest,
		Counter:          0,
		UnackedDatagrams: make(map[uint64]time.Time),
	}
	h.senderStates[key] = state
	return state
}

// shouldUseDatagram2 determines if the next message should use datagram2.
// Returns true when ANY of these conditions are met:
//   1. First message (counter == 0)
//   2. Counter-based trigger (counter % RepliableInterval == 0)
//   3. Time-based trigger (time since last datagram2 >= RepliableTimeInterval)
//
// The time-based trigger ensures hash mappings don't expire on low-traffic connections.
func (s *SenderState) shouldUseDatagram2() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check counter-based trigger
	if s.Counter%RepliableInterval == FirstMessageIndex {
		return true
	}

	// Check time-based trigger
	// Zero value LastDatagram2Time means we've never sent datagram2, so force it
	if s.LastDatagram2Time.IsZero() {
		return true
	}

	// If enough time has elapsed, use datagram2 to refresh hash mapping
	if time.Since(s.LastDatagram2Time) >= RepliableTimeInterval {
		return true
	}

	return false
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

// markDatagram2Sent updates tracking when a datagram2 is sent.
// Records the timestamp and increments the sequence number.
// Returns the sequence number assigned to this datagram2.
func (s *SenderState) markDatagram2Sent() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.LastDatagram2Time = time.Now()
	s.LastDatagram2SeqNum++
	return s.LastDatagram2SeqNum
}

// shouldRequestAck determines if an ACK should be requested for this datagram2.
// Returns true when:
//   - RTT is not yet measured (RTT == 0)
//   - Every AckRequestInterval datagram2
//   - Approaching unacked limit
func (s *SenderState) shouldRequestAck() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Always request ACK if we don't have RTT measurement yet
	if s.RTT == 0 {
		return true
	}

	// Request ACK every AckRequestInterval datagram2 messages
	if s.LastDatagram2SeqNum%uint64(AckRequestInterval) == 0 {
		return true
	}

	// Request ACK if approaching unacked limit
	if len(s.UnackedDatagrams) >= MaxUnackedDatagrams-1 {
		return true
	}

	return false
}

// trackUnackedDatagram records a datagram2 for ACK tracking.
func (s *SenderState) trackUnackedDatagram(seqNum uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.UnackedDatagrams[seqNum] = time.Now()
}

// processAck handles an ACK response for a specific sequence number.
// Calculates RTT, updates PathQuality, and cleans up tracking state.
func (s *SenderState) processAck(ackedSeqNum uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Look up the send timestamp
	sendTime, exists := s.UnackedDatagrams[ackedSeqNum]
	if !exists {
		// ACK for unknown or already processed sequence number
		return
	}

	// Calculate RTT in milliseconds
	rtt := time.Since(sendTime).Milliseconds()
	if rtt < 0 {
		rtt = 0 // Shouldn't happen, but be defensive
	}

	// Update RTT using exponential moving average: RTT = (7×RTT + sample)/8
	if s.RTT == 0 {
		s.RTT = uint64(rtt) // First measurement
	} else {
		s.RTT = (7*s.RTT + uint64(rtt)) / 8
	}

	// Update PathQuality positively (exponential moving average)
	// quality = 0.9 × quality + 0.1 × 1.0
	if s.PathQuality == 0.0 {
		s.PathQuality = 1.0 // First ACK
	} else {
		s.PathQuality = 0.9*s.PathQuality + 0.1*1.0
	}

	// Update last acked sequence number
	if ackedSeqNum > s.LastAckedSeqNum {
		s.LastAckedSeqNum = ackedSeqNum
	}

	// Remove this and older entries from unacked map (implicit ACK)
	for seq := range s.UnackedDatagrams {
		if seq <= ackedSeqNum {
			delete(s.UnackedDatagrams, seq)
		}
	}
}

// cleanupStaleUnacked removes unacked datagrams older than AckTimeout.
// Returns the count of timed-out messages.
func (s *SenderState) cleanupStaleUnacked() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	timedOut := 0

	for seqNum, sendTime := range s.UnackedDatagrams {
		if now.Sub(sendTime) > AckTimeout {
			delete(s.UnackedDatagrams, seqNum)
			timedOut++
		}
	}

	// Degrade PathQuality for each timeout
	// quality = 0.9 × quality + 0.1 × 0.0
	for i := 0; i < timedOut; i++ {
		if s.PathQuality > 0.0 {
			s.PathQuality = 0.9 * s.PathQuality
		}
	}

	return timedOut
}

// resetCounter resets the counter to a specific value.
// Used after forced datagram2 sends to reset the cycle.
func (s *SenderState) resetCounter(value uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Counter = value
	s.LastSendTime = time.Now()
}

// encodeAckRequest encodes datagram2 payload with ACK request.
// Format: [marker:1 byte][seqNum:8 bytes][original data]
func encodeAckRequest(seqNum uint64, data []byte) []byte {
	payload := make([]byte, 1+8+len(data))
	payload[0] = AckRequestMarker
	binary.BigEndian.PutUint64(payload[1:9], seqNum)
	copy(payload[9:], data)
	return payload
}

// encodeAckResponse encodes an ACK response message.
// Format: [marker:1 byte][ackedSeqNum:8 bytes]
func encodeAckResponse(ackedSeqNum uint64) []byte {
	payload := make([]byte, 9)
	payload[0] = AckResponseMarker
	binary.BigEndian.PutUint64(payload[1:9], ackedSeqNum)
	return payload
}

// decodeAckMessage attempts to decode an ACK-related message.
// Returns: (isAckRequest bool, isAckResponse bool, seqNum uint64, data []byte)
func decodeAckMessage(payload []byte) (bool, bool, uint64, []byte) {
	if len(payload) < 9 {
		// Not an ACK message, return original data
		return false, false, 0, payload
	}

	marker := payload[0]
	if marker == AckRequestMarker {
		// ACK request: extract seqNum and original data
		seqNum := binary.BigEndian.Uint64(payload[1:9])
		data := payload[9:]
		return true, false, seqNum, data
	} else if marker == AckResponseMarker {
		// ACK response: extract acked seqNum
		seqNum := binary.BigEndian.Uint64(payload[1:9])
		return false, true, seqNum, nil
	}

	// Regular data message
	return false, false, 0, payload
}

// SendDatagram sends data to the specified destination using the hybrid protocol.
// It automatically chooses between datagram2 and datagram3 based on the send counter and time:
//   - First message (counter=0) uses datagram2
//   - Every RepliableInterval-th message uses datagram2
//   - When RepliableTimeInterval has elapsed, uses datagram2 (prevents hash expiry)
//   - All other messages use datagram3 for lower overhead
//
// This approach provides sender identification through periodic authentication
// while minimizing overhead for bulk traffic and preventing hash mapping expiry.
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
		// Mark that we're sending datagram2 and get sequence number
		seqNum := state.markDatagram2Sent()

		// Determine if we should request ACK
		var payload []byte
		if state.shouldRequestAck() {
			// Encode with ACK request
			payload = encodeAckRequest(seqNum, data)
			// Track this datagram for ACK matching
			state.trackUnackedDatagram(seqNum)
		} else {
			// Regular datagram2 without ACK request
			payload = data
		}

		// Send authenticated datagram2 for identity establishment/refresh
		if err := h.datagram2Sub.SendDatagram(payload, dest); err != nil {
			return fmt.Errorf("hybrid2 datagram2 send: %w", err)
		}
	} else {
		// Send low-overhead datagram3 for bulk traffic
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

	// Mark datagram2 sent and get sequence number
	seqNum := state.markDatagram2Sent()

	// Send authenticated datagram2 (without ACK request for forced sends)
	if err := h.datagram2Sub.SendDatagram(data, dest); err != nil {
		return fmt.Errorf("hybrid2 forced datagram2 send: %w", err)
	}

	// Reset counter to 1 (just sent message 0, next will be 1)
	state.resetCounter(1)

	return nil
}

// sendAckResponse sends an ACK response for a received datagram2.
// This is called internally when processing ACK requests.
func (h *HybridSession) sendAckResponse(ackedSeqNum uint64, dest i2pkeys.I2PAddr) error {
	// Check if session is closed
	h.closeMu.RLock()
	if h.closed {
		h.closeMu.RUnlock()
		return ErrSessionClosed
	}
	h.closeMu.RUnlock()

	// Encode ACK response
	payload := encodeAckResponse(ackedSeqNum)

	// Send as datagram2 (ACK responses are authenticated messages)
	if err := h.datagram2Sub.SendDatagram(payload, dest); err != nil {
		return fmt.Errorf("hybrid2 ACK response send: %w", err)
	}

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
