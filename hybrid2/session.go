package hybrid2

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/go-i2p/go-sam-go/common"
	"github.com/go-i2p/go-sam-go/primary"
	"github.com/go-i2p/i2pkeys"
)

// SessionOption is a function that configures a HybridSession.
type SessionOption func(*HybridSession)

// WithOptions sets SAM session options for the subsessions.
func WithOptions(options []string) SessionOption {
	return func(s *HybridSession) {
		s.options = options
	}
}

// NewHybridSession creates a new HybridSession from a PrimarySession.
// The hybrid session creates both datagram2 and datagram3 subsessions
// for efficient hybrid protocol communication.
//
// The session automatically manages:
//   - Sender state (counter tracking per destination)
//   - Receiver state (hash-to-destination mappings)
//   - Background receive loops with channel multiplexing
//   - Cleanup of expired mappings
//
// Example usage:
//
//	primary, err := primary.NewPrimarySession(sam, "my-session", keys, nil)
//	if err != nil {
//	    // handle error
//	}
//	hybrid, err := NewHybridSession(primary, "hybrid", WithOptions([]string{"inbound.length=1"}))
//	if err != nil {
//	    // handle error
//	}
//	defer hybrid.Close()
func NewHybridSession(primarySession *primary.PrimarySession, id string, opts ...SessionOption) (*HybridSession, error) {
	session := &HybridSession{
		primary:          primarySession,
		id:               id,
		senderStates:     make(map[string]*SenderState),
		receiverMap:      make(map[SenderHash]*ReceiverMapping),
		cleanupDone:      make(chan struct{}),
		ackCleanupStop:   make(chan struct{}),
		ackCleanupTicker: time.NewTicker(AckCleanupInterval),
	}

	// Apply options
	for _, opt := range opts {
		opt(session)
	}

	// Create datagram2 subsession for authenticated messaging
	datagram2SubID := id + "-dg2"
	datagram2Sub, err := primarySession.NewDatagramSubSession(datagram2SubID, session.options)
	if err != nil {
		return nil, fmt.Errorf("creating datagram2 subsession: %w", err)
	}
	session.datagram2Sub = datagram2Sub

	// Create datagram3 subsession for low-overhead bulk messaging
	datagram3SubID := id + "-dg3"
	datagram3Sub, err := primarySession.NewDatagram3SubSession(datagram3SubID, session.options)
	if err != nil {
		// Clean up the datagram2 subsession on failure
		datagram2Sub.Close()
		return nil, fmt.Errorf("creating datagram3 subsession: %w", err)
	}
	session.datagram3Sub = datagram3Sub

	// Set local address from primary session
	session.localAddr = primarySession.Addr()

	// Initialize receiver infrastructure
	if err := session.initializeReceivers(); err != nil {
		session.closeSubsessions()
		return nil, fmt.Errorf("initializing receivers: %w", err)
	}

	// Start background receive loops
	session.startReceiveLoops()

	// Start ACK cleanup goroutine
	go session.ackCleanupLoop()

	return session, nil
}

// NewHybridSessionFromSAM creates a new HybridSession with a fresh PrimarySession.
// This is a convenience function that creates the primary session automatically.
//
// Example usage:
//
//	hybrid, err := NewHybridSessionFromSAM(sam, "my-hybrid", keys, nil)
//	if err != nil {
//	    // handle error
//	}
//	defer hybrid.Close()
func NewHybridSessionFromSAM(sam *common.SAM, id string, keys i2pkeys.I2PKeys, opts ...SessionOption) (*HybridSession, error) {
	// Create primary session
	primarySession, err := primary.NewPrimarySession(sam, id, keys, nil)
	if err != nil {
		return nil, fmt.Errorf("creating primary session: %w", err)
	}

	// Create hybrid session using the primary
	session, err := NewHybridSession(primarySession, id+"-hybrid", opts...)
	if err != nil {
		primarySession.Close()
		return nil, err
	}

	// Store SAM reference
	session.sam = sam

	return session, nil
}

// closeSubsessions closes both datagram subsessions.
func (h *HybridSession) closeSubsessions() {
	if h.datagram2Sub != nil {
		h.datagram2Sub.Close()
	}
	if h.datagram3Sub != nil {
		h.datagram3Sub.Close()
	}
}

// ackCleanupLoop periodically scans all sender states and removes stale unacked messages.
// This runs in a background goroutine for the lifetime of the session.
func (h *HybridSession) ackCleanupLoop() {
	for {
		select {
		case <-h.ackCleanupTicker.C:
			h.cleanupStaleUnacked()
		case <-h.ackCleanupStop:
			h.ackCleanupTicker.Stop()
			return
		}
	}
}

// cleanupStaleUnacked scans all sender states and cleans up stale unacked messages.
func (h *HybridSession) cleanupStaleUnacked() {
	h.senderMu.RLock()
	states := make([]*SenderState, 0, len(h.senderStates))
	for _, state := range h.senderStates {
		states = append(states, state)
	}
	h.senderMu.RUnlock()

	// Clean each state
	for _, state := range states {
		state.cleanupStaleUnacked()
	}
}

// Close closes the hybrid session and all associated resources.
// This method:
//   - Stops receive loops
//   - Cleans up receiver state
//   - Closes both datagram subsessions
//   - Marks the session as closed
//
// After Close() is called, all operations on this session will return ErrSessionClosed.
func (h *HybridSession) Close() error {
	h.closeMu.Lock()
	if h.closed {
		h.closeMu.Unlock()
		return nil // Already closed
	}
	h.closed = true
	h.closeMu.Unlock()

	// Stop receive loops and cleanup
	h.stopReceiveLoops()

	// Stop ACK cleanup goroutine
	close(h.ackCleanupStop)

	// Close subsessions
	h.closeSubsessions()

	// Signal cleanup is done
	close(h.cleanupDone)

	return nil
}

// ID returns the unique identifier for this hybrid session.
func (h *HybridSession) ID() string {
	return h.id
}

// Addr returns the local I2P address for this session.
func (h *HybridSession) Addr() i2pkeys.I2PAddr {
	return h.localAddr
}

// IsClosed returns whether this session has been closed.
func (h *HybridSession) IsClosed() bool {
	h.closeMu.RLock()
	defer h.closeMu.RUnlock()
	return h.closed
}

// Datagram2Sub returns the underlying datagram2 subsession.
// Use with caution - direct access bypasses hybrid protocol logic.
func (h *HybridSession) Datagram2Sub() *primary.DatagramSubSession {
	return h.datagram2Sub
}

// Datagram3Sub returns the underlying datagram3 subsession.
// Use with caution - direct access bypasses hybrid protocol logic.
func (h *HybridSession) Datagram3Sub() *primary.Datagram3SubSession {
	return h.datagram3Sub
}

// GetPathStats returns path quality statistics for a specific destination.
// Returns nil if no state exists for the destination.
func (h *HybridSession) GetPathStats(dest i2pkeys.I2PAddr) *PathStats {
	key := dest.Base64()

	h.senderMu.RLock()
	state, exists := h.senderStates[key]
	h.senderMu.RUnlock()

	if !exists {
		return nil
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	// Find the timestamp of the last ACK
	var lastAcked time.Time
	for _, sendTime := range state.UnackedDatagrams {
		// If we have unacked messages, last ACK was before the oldest unacked
		if sendTime.Before(lastAcked) || lastAcked.IsZero() {
			lastAcked = sendTime
		}
	}
	// If all are acked, last ACK was recent
	if len(state.UnackedDatagrams) == 0 && !state.LastDatagram2Time.IsZero() {
		lastAcked = state.LastDatagram2Time
	}

	return &PathStats{
		Destination:   dest,
		RTT:           state.RTT,
		PathQuality:   state.PathQuality,
		UnackedCount:  len(state.UnackedDatagrams),
		LastDatagram2: state.LastDatagram2Time,
		LastAcked:     lastAcked,
		TotalSent:     state.LastDatagram2SeqNum,
		TotalAcked:    state.LastAckedSeqNum,
	}
}

// GetAllPathStats returns path statistics for all known destinations.
// Useful for monitoring overall mesh network quality.
func (h *HybridSession) GetAllPathStats() []*PathStats {
	h.senderMu.RLock()
	states := make([]*SenderState, 0, len(h.senderStates))
	for _, state := range h.senderStates {
		states = append(states, state)
	}
	h.senderMu.RUnlock()

	result := make([]*PathStats, 0, len(states))
	for _, state := range states {
		if stats := h.GetPathStats(state.Destination); stats != nil {
			result = append(result, stats)
		}
	}

	return result
}

// PacketConn returns a net.PacketConn interface for this session.
// This allows the hybrid session to be used with standard Go networking patterns.
//
// Multiple PacketConn wrappers can be obtained from the same session.
// Closing a PacketConn marks only that wrapper as closed; the underlying
// session remains active. Call session.Close() to close the session itself.
func (h *HybridSession) PacketConn() net.PacketConn {
	return &HybridPacketConn{session: h}
}

// ErrPacketConnClosed is returned when operations are attempted on a closed PacketConn.
var ErrPacketConnClosed = errors.New("hybrid2: packet connection is closed")

// isClosed checks if this PacketConn wrapper has been closed.
func (c *HybridPacketConn) isClosed() bool {
	c.closedMu.RLock()
	defer c.closedMu.RUnlock()
	return c.closed
}

// ReadFrom implements net.PacketConn.ReadFrom.
// It receives a datagram and returns the data and source address.
// Respects the read deadline set via SetReadDeadline or SetDeadline.
func (c *HybridPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	// Check if this PacketConn is closed
	if c.isClosed() {
		return 0, nil, ErrPacketConnClosed
	}

	// Check for deadline
	c.deadlineMu.RLock()
	deadline := c.readDeadline
	c.deadlineMu.RUnlock()

	var dg *HybridDatagram

	if deadline.IsZero() {
		// No deadline, block indefinitely
		dg, err = c.session.ReceiveDatagram()
	} else {
		// Calculate timeout duration
		timeout := time.Until(deadline)
		if timeout <= 0 {
			return 0, nil, ErrTimeout
		}

		// Use ReceiveDatagramTimeout for deadline support
		dg, err = c.session.ReceiveDatagramTimeout(timeout)
	}

	if err != nil {
		return 0, nil, err
	}

	n = copy(p, dg.Data)
	addr = &HybridAddr{dg.Source}
	return n, addr, nil
}

// WriteTo implements net.PacketConn.WriteTo.
// It sends a datagram using the hybrid protocol.
// Respects the write deadline set via SetWriteDeadline or SetDeadline.
func (c *HybridPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	// Check if this PacketConn is closed
	if c.isClosed() {
		return 0, ErrPacketConnClosed
	}

	// Extract I2P address from the net.Addr
	var dest i2pkeys.I2PAddr
	switch a := addr.(type) {
	case *HybridAddr:
		dest = a.I2PAddr
	case i2pkeys.I2PAddr:
		dest = a
	default:
		// Try to parse as string
		dest = i2pkeys.I2PAddr(addr.String())
	}

	// Check for write deadline
	c.deadlineMu.RLock()
	deadline := c.writeDeadline
	c.deadlineMu.RUnlock()

	if deadline.IsZero() {
		// No deadline, send without timeout
		if err := c.session.SendDatagram(p, dest); err != nil {
			return 0, err
		}
	} else {
		// Calculate timeout duration from deadline
		timeout := time.Until(deadline)
		if timeout <= 0 {
			return 0, ErrTimeout
		}

		// Use timeout-aware send
		if err := c.session.SendDatagramTimeout(p, dest, timeout); err != nil {
			return 0, err
		}
	}

	return len(p), nil
}

// Close implements net.PacketConn.Close.
// This closes only this PacketConn wrapper, not the underlying HybridSession.
// To close the session itself, call session.Close() directly.
// After Close() is called, ReadFrom and WriteTo will return ErrPacketConnClosed.
func (c *HybridPacketConn) Close() error {
	c.closedMu.Lock()
	defer c.closedMu.Unlock()
	c.closed = true
	return nil
}

// LocalAddr implements net.PacketConn.LocalAddr.
func (c *HybridPacketConn) LocalAddr() net.Addr {
	return &HybridAddr{c.session.localAddr}
}

// SetDeadline implements net.PacketConn.SetDeadline.
// Sets both read and write deadlines for this connection.
func (c *HybridPacketConn) SetDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	c.readDeadline = t
	c.writeDeadline = t
	return nil
}

// SetReadDeadline implements net.PacketConn.SetReadDeadline.
// Sets the deadline for future ReadFrom calls.
// A zero value for t means ReadFrom will not time out.
func (c *HybridPacketConn) SetReadDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	c.readDeadline = t
	return nil
}

// SetWriteDeadline implements net.PacketConn.SetWriteDeadline.
// Sets the deadline for future WriteTo calls.
// A zero value for t means WriteTo will not time out.
func (c *HybridPacketConn) SetWriteDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	c.writeDeadline = t
	return nil
}

// SenderStateCount returns the number of destination states being tracked.
func (h *HybridSession) SenderStateCount() int {
	h.senderMu.RLock()
	defer h.senderMu.RUnlock()
	return len(h.senderStates)
}

// ReceiverMappingCount returns the number of sender hash mappings.
func (h *HybridSession) ReceiverMappingCount() int {
	if h.receiverState == nil {
		return 0
	}
	return h.receiverState.MappingCount()
}

// WaitForClose blocks until the session is fully closed.
func (h *HybridSession) WaitForClose() {
	<-h.cleanupDone
}

// Ensure HybridPacketConn implements net.PacketConn
var _ net.PacketConn = (*HybridPacketConn)(nil)

// Ensure HybridSession implements the interfaces
var _ HybridSender = (*HybridSession)(nil)

// Compile-time check for HybridReceiver (requires ReceiveDatagram and LookupSender)
// Note: ReceiveDatagram is in receiver.go, LookupSender is in receiver.go
type hybridReceiverCheck interface {
	ReceiveDatagram() (*HybridDatagram, error)
	LookupSender(hash SenderHash) (i2pkeys.I2PAddr, bool)
}

var _ hybridReceiverCheck = (*HybridSession)(nil)

// mu protects concurrent access during cleanup operations
var cleanupMu sync.Mutex
