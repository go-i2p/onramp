package hybrid2

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/go-i2p/go-sam-go/datagram"
	"github.com/go-i2p/go-sam-go/datagram3"
	"github.com/go-i2p/i2pkeys"
)

// ComputeSenderHash computes a 32-byte SHA-256 hash from an I2P destination address.
// This hash is used by datagram3 for sender identification and can be used to
// map datagram3 sources to their full destinations (established via datagram2).
func ComputeSenderHash(addr i2pkeys.I2PAddr) SenderHash {
	var hash SenderHash
	h := sha256.New()
	if bytes, err := addr.ToBytes(); err == nil {
		h.Write(bytes)
		copy(hash[:], h.Sum(nil))
	}
	return hash
}

// ReceiverMappingEntry stores a mapping from sender hash to full destination address.
// This mapping is established when receiving datagram2 messages and used to resolve
// datagram3 sources.
type ReceiverMappingEntry struct {
	// Hash is the 32-byte sender hash (derived from datagram2 source)
	Hash SenderHash
	// Destination is the full I2P address of the sender
	Destination i2pkeys.I2PAddr
	// FirstSeen is when this sender was first observed
	FirstSeen time.Time
	// LastRefresh is when this mapping was last updated by a datagram2
	LastRefresh time.Time
	// mu protects field access
	mu sync.RWMutex
}

// isExpired checks if this mapping has expired and should be removed.
func (r *ReceiverMappingEntry) isExpired() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return time.Since(r.LastRefresh) > HashExpiryDuration
}

// refresh updates the LastRefresh time for this mapping.
func (r *ReceiverMappingEntry) refresh() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.LastRefresh = time.Now()
}

// ReceiverState manages the hash-to-destination mapping table for receiving.
type ReceiverState struct {
	// mappings maps sender hashes to their full destination addresses
	mappings map[SenderHash]*ReceiverMappingEntry
	// mu protects the mappings map
	mu sync.RWMutex
	// cleanupTicker runs periodic cleanup of expired mappings
	cleanupTicker *time.Ticker
	// stopCleanup signals the cleanup goroutine to stop
	stopCleanup chan struct{}
}

// NewReceiverState creates a new ReceiverState with initialized maps and
// starts the background cleanup goroutine.
func NewReceiverState() *ReceiverState {
	rs := &ReceiverState{
		mappings:      make(map[SenderHash]*ReceiverMappingEntry),
		cleanupTicker: time.NewTicker(HashExpiryDuration / 2), // Cleanup at half the expiry interval
		stopCleanup:   make(chan struct{}),
	}
	go rs.cleanupLoop()
	return rs
}

// cleanupLoop periodically removes expired mappings from the table.
func (rs *ReceiverState) cleanupLoop() {
	for {
		select {
		case <-rs.cleanupTicker.C:
			rs.removeExpiredMappings()
		case <-rs.stopCleanup:
			rs.cleanupTicker.Stop()
			return
		}
	}
}

// removeExpiredMappings removes all mappings that have exceeded HashExpiryDuration.
func (rs *ReceiverState) removeExpiredMappings() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	for hash, entry := range rs.mappings {
		if entry.isExpired() {
			delete(rs.mappings, hash)
		}
	}
}

// Stop stops the cleanup goroutine. Should be called when the session closes.
func (rs *ReceiverState) Stop() {
	close(rs.stopCleanup)
}

// RegisterSender records or refreshes a mapping from sender hash to full destination.
// This should be called when receiving a datagram2 message.
func (rs *ReceiverState) RegisterSender(hash SenderHash, dest i2pkeys.I2PAddr) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if entry, exists := rs.mappings[hash]; exists {
		entry.refresh()
		return
	}

	now := time.Now()
	rs.mappings[hash] = &ReceiverMappingEntry{
		Hash:        hash,
		Destination: dest,
		FirstSeen:   now,
		LastRefresh: now,
	}
}

// LookupSender retrieves the full destination address for a sender hash.
// Returns the destination and true if found, or empty address and false if not found.
func (rs *ReceiverState) LookupSender(hash SenderHash) (i2pkeys.I2PAddr, bool) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	entry, exists := rs.mappings[hash]
	if !exists || entry.isExpired() {
		return i2pkeys.I2PAddr(""), false
	}
	return entry.Destination, true
}

// MappingCount returns the current number of active sender mappings.
func (rs *ReceiverState) MappingCount() int {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return len(rs.mappings)
}

// datagram2Receiver handles receiving from the datagram2 subsession.
type datagram2Receiver struct {
	reader *datagram.DatagramReader
	closed bool
	mu     sync.RWMutex
}

// datagram3Receiver handles receiving from the datagram3 subsession.
type datagram3Receiver struct {
	reader *datagram3.Datagram3Reader
	closed bool
	mu     sync.RWMutex
}

// initializeReceivers sets up the receive infrastructure for the hybrid session.
// This creates readers for both datagram2 and datagram3 subsessions and starts
// their receive loops.
func (h *HybridSession) initializeReceivers() error {
	// Initialize receiver state for hash mapping
	h.receiverState = NewReceiverState()

	// Create readers for both subsessions
	h.datagram2Reader = h.datagram2Sub.NewReader()
	h.datagram3Reader = h.datagram3Sub.NewReader()

	// Start receive loops in background goroutines
	// These are started by the session management layer (session.go)
	// and feed into the multiplexed receive channel

	return nil
}

// startReceiveLoops starts background goroutines to receive from both subsessions
// and multiplex the results into a single channel.
func (h *HybridSession) startReceiveLoops() {
	h.recvChan = make(chan *HybridDatagram, RecvChanBufferSize)
	h.recvErrChan = make(chan error, 1)
	h.recvStopChan = make(chan struct{})

	// Start datagram2 receive loop
	go h.datagram2ReceiveLoop()

	// Start datagram3 receive loop
	go h.datagram3ReceiveLoop()
}

// reportRecvError sends a receive error to the error channel non-blocking.
func (h *HybridSession) reportRecvError(err error) {
	select {
	case h.recvErrChan <- err:
	default:
		// Error channel full, skip
	}
}

// datagram2ReceiveLoop continuously receives datagram2 messages and processes them.
// When a datagram2 is received, it updates the sender mapping and forwards the
// message to the unified receive channel.
func (h *HybridSession) datagram2ReceiveLoop() {
	for {
		select {
		case <-h.recvStopChan:
			return
		default:
			if stop := h.receiveDatagram2(); stop {
				return
			}
		}
	}
}

// receiveDatagram2 performs a single datagram2 receive iteration.
// Returns true if the loop should stop.
func (h *HybridSession) receiveDatagram2() bool {
	dg, err := h.datagram2Sub.ReceiveDatagram()
	if err != nil {
		select {
		case <-h.recvStopChan:
			return true
		default:
			h.reportRecvError(fmt.Errorf("datagram2 receive: %w", err))
			return false
		}
	}

	hash := ComputeSenderHash(dg.Source)
	h.receiverState.RegisterSender(hash, dg.Source)

	data, forward := h.processDatagram2Ack(dg.Source, dg.Data)
	if !forward {
		return false
	}

	hybrid := &HybridDatagram{
		Data:         data,
		Source:       dg.Source,
		SourceHash:   hash,
		WasDatagram2: true,
		Timestamp:    time.Now(),
	}

	select {
	case h.recvChan <- hybrid:
	case <-h.recvStopChan:
		return true
	}
	return false
}

// processDatagram2Ack handles ACK message processing for a received datagram2.
// Returns the payload data and true if the datagram should be forwarded to the application.
func (h *HybridSession) processDatagram2Ack(source i2pkeys.I2PAddr, rawData []byte) ([]byte, bool) {
	isAckReq, isAckResp, seqNum, data := decodeAckMessage(rawData)

	if isAckResp {
		state := h.getSenderState(source)
		state.processAck(seqNum)
		return nil, false
	}

	if isAckReq {
		if err := h.sendAckResponse(seqNum, source); err != nil {
			h.reportRecvError(fmt.Errorf("ACK response send: %w", err))
		}
	}

	return data, true
}

// datagram3ReceiveLoop continuously receives datagram3 messages and processes them.
// When a datagram3 is received, it attempts to resolve the sender hash using the
// mapping table and forwards the message to the unified receive channel.
func (h *HybridSession) datagram3ReceiveLoop() {
	reader := h.datagram3Reader

	for {
		select {
		case <-h.recvStopChan:
			reader.Close()
			return
		default:
			if stop := h.receiveDatagram3(reader); stop {
				reader.Close()
				return
			}
		}
	}
}

// receiveDatagram3 performs a single datagram3 receive iteration.
// Returns true if the loop should stop.
func (h *HybridSession) receiveDatagram3(reader *datagram3.Datagram3Reader) bool {
	dg, err := reader.ReceiveDatagram()
	if err != nil {
		select {
		case <-h.recvStopChan:
			return true
		default:
			h.reportRecvError(fmt.Errorf("datagram3 receive: %w", err))
			return false
		}
	}

	var hash SenderHash
	copy(hash[:], dg.SourceHash[:])

	source := h.lookupOrResolveDatagram3Source(dg, hash)

	hybrid := &HybridDatagram{
		Data:         dg.Data,
		Source:       source,
		SourceHash:   hash,
		WasDatagram2: false,
		Timestamp:    time.Now(),
	}

	select {
	case h.recvChan <- hybrid:
	case <-h.recvStopChan:
		return true
	}
	return false
}

// lookupOrResolveDatagram3Source resolves a datagram3 sender from the hash mapping table,
// falling back to SAM bridge resolution when the hash is not cached.
func (h *HybridSession) lookupOrResolveDatagram3Source(dg *datagram3.Datagram3, hash SenderHash) i2pkeys.I2PAddr {
	if dest, found := h.receiverState.LookupSender(hash); found {
		return dest
	}
	if err := dg.ResolveSource(h.datagram3Sub.Datagram3Session); err == nil {
		h.receiverState.RegisterSender(hash, dg.Source)
		return dg.Source
	}
	return i2pkeys.I2PAddr("")
}

// ReceiveDatagram receives the next datagram from either subsession.
// The returned HybridDatagram includes source information resolved from the
// mapping table when available. Check WasDatagram2 to determine if the source
// is authenticated (true) or derived from hash lookup (false).
func (h *HybridSession) ReceiveDatagram() (*HybridDatagram, error) {
	h.closeMu.RLock()
	if h.closed {
		h.closeMu.RUnlock()
		return nil, ErrSessionClosed
	}
	h.closeMu.RUnlock()

	select {
	case dg := <-h.recvChan:
		return dg, nil
	case err := <-h.recvErrChan:
		return nil, err
	}
}

// ReceiveDatagramTimeout receives the next datagram with a timeout.
// Returns ErrTimeout if the timeout expires before a datagram is received.
func (h *HybridSession) ReceiveDatagramTimeout(timeout time.Duration) (*HybridDatagram, error) {
	h.closeMu.RLock()
	if h.closed {
		h.closeMu.RUnlock()
		return nil, ErrSessionClosed
	}
	h.closeMu.RUnlock()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case dg := <-h.recvChan:
		return dg, nil
	case err := <-h.recvErrChan:
		return nil, err
	case <-timer.C:
		return nil, ErrTimeout
	}
}

// stopReceiveLoops signals the receive loops to stop and waits for cleanup.
func (h *HybridSession) stopReceiveLoops() {
	// Signal loops to stop
	close(h.recvStopChan)

	// Stop the receiver state cleanup goroutine
	if h.receiverState != nil {
		h.receiverState.Stop()
	}

	// Close readers if they exist
	if h.datagram2Reader != nil {
		h.datagram2Reader.Close()
	}
	if h.datagram3Reader != nil {
		h.datagram3Reader.Close()
	}
}

// LookupSender retrieves the full destination address for a sender hash.
// This can be used to look up senders from datagram3 messages.
func (h *HybridSession) LookupSender(hash SenderHash) (i2pkeys.I2PAddr, bool) {
	if h.receiverState == nil {
		return i2pkeys.I2PAddr(""), false
	}
	return h.receiverState.LookupSender(hash)
}
