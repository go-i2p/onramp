package hybrid1

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-i2p/go-sam-go/common"
	"github.com/go-i2p/go-sam-go/primary"
	"github.com/go-i2p/i2pkeys"
)

// SessionOption is a function that configures a Hybrid1Session.
type SessionOption func(*Hybrid1Session)

// WithOptions sets SAM session options for the subsession.
func WithOptions(options []string) SessionOption {
	return func(s *Hybrid1Session) {
		s.options = options
	}
}

// WithLocalPort sets the local port for the session.
func WithLocalPort(port uint16) SessionOption {
	return func(s *Hybrid1Session) {
		s.localPort = port
	}
}

// NewHybrid1Session creates a new Hybrid1Session from a PrimarySession.
// The session wraps SAM3 datagram operations with i2pd-compatible protocol framing.
//
// Example usage:
//
//	primary, err := primary.NewPrimarySession(sam, "my-session", keys, nil)
//	if err != nil {
//	    // handle error
//	}
//	hybrid1, err := NewHybrid1Session(primary, "hybrid1", WithOptions([]string{"inbound.length=1"}))
//	if err != nil {
//	    // handle error
//	}
//	defer hybrid1.Close()
func NewHybrid1Session(primarySession *primary.PrimarySession, id string, opts ...SessionOption) (*Hybrid1Session, error) {
	session := &Hybrid1Session{
		primary:      primarySession,
		id:           id,
		senderStates: make(map[string]*SenderState),
		receiverMap:  make(map[SenderHash]*ReceiverMapping),
		recvChan:     make(chan *Hybrid1Datagram, DefaultReceiveChannelSize),
		recvErrChan:  make(chan error, 10),
		recvStopChan: make(chan struct{}),
		cleanupDone:  make(chan struct{}),
	}

	// Apply options.
	for _, opt := range opts {
		opt(session)
	}

	// Create datagram subsession for i2pd-compatible communication.
	datagramSubID := id + "-dg"
	var err error
	session.datagramSub, err = primarySession.NewDatagramSubSession(datagramSubID, session.options)
	if err != nil {
		return nil, fmt.Errorf("hybrid1: creating datagram subsession: %w", err)
	}

	// Set local address from primary session.
	session.localAddr = primarySession.Addr()

	// Start background receive loop.
	go session.receiveLoop()

	// Start cleanup goroutine.
	go session.cleanupLoop()

	return session, nil
}

// NewHybrid1SessionFromSAM creates a new Hybrid1Session with a fresh PrimarySession.
func NewHybrid1SessionFromSAM(sam *common.SAM, id string, keys i2pkeys.I2PKeys, opts ...SessionOption) (*Hybrid1Session, error) {
	// Create primary session.
	primarySession, err := primary.NewPrimarySession(sam, id, keys, nil)
	if err != nil {
		return nil, fmt.Errorf("hybrid1: creating primary session: %w", err)
	}

	// Create hybrid1 session using the primary.
	session, err := NewHybrid1Session(primarySession, id+"-hybrid1", opts...)
	if err != nil {
		primarySession.Close()
		return nil, err
	}

	return session, nil
}

// receiveLoop continuously reads datagrams and processes them.
func (h *Hybrid1Session) receiveLoop() {
	for {
		select {
		case <-h.recvStopChan:
			return
		default:
			// Read directly from datagram subsession.
			dg, err := h.datagramSub.ReceiveDatagram()
			if err != nil {
				// Check if session is closed.
				h.closeMu.RLock()
				closed := h.closed
				h.closeMu.RUnlock()
				if closed {
					return
				}

				// Send error to error channel (non-blocking).
				select {
				case h.recvErrChan <- err:
				default:
				}
				continue
			}

			// Process the received datagram with i2pd framing.
			hybrid1Dg := h.processReceivedDatagram(dg.Data, dg.Source)

			// Send to receive channel.
			select {
			case h.recvChan <- hybrid1Dg:
			case <-h.recvStopChan:
				return
			}
		}
	}
}

// processReceivedDatagram extracts i2pd header and creates a Hybrid1Datagram.
func (h *Hybrid1Session) processReceivedDatagram(data []byte, source i2pkeys.I2PAddr) *Hybrid1Datagram {
	dg := &Hybrid1Datagram{
		Source:    source,
		Timestamp: time.Now(),
	}

	// Extract i2pd header if present.
	if len(data) >= I2PD_HEADER_SIZE {
		dg.SourcePort = uint16(data[0])<<8 | uint16(data[1])
		dg.DestPort = uint16(data[2])<<8 | uint16(data[3])
		dg.Data = data[I2PD_HEADER_SIZE:]
	} else {
		// No header, use raw data.
		dg.Data = data
		dg.SourcePort = I2PD_DEFAULT_PORT
		dg.DestPort = I2PD_DEFAULT_PORT
	}

	// Update receiver mapping.
	h.updateReceiverMapping(source, dg.SourcePort)

	return dg
}

// updateReceiverMapping updates or creates a mapping for the sender.
func (h *Hybrid1Session) updateReceiverMapping(source i2pkeys.I2PAddr, port uint16) {
	hash := computeSenderHash(source)

	h.receiverMu.Lock()
	defer h.receiverMu.Unlock()

	mapping, exists := h.receiverMap[hash]
	if !exists {
		mapping = &ReceiverMapping{
			Hash:        hash,
			Destination: source,
			Port:        port,
			FirstSeen:   time.Now(),
			LastRefresh: time.Now(),
		}
		h.receiverMap[hash] = mapping
	} else {
		mapping.mu.Lock()
		mapping.LastRefresh = time.Now()
		mapping.Port = port
		mapping.mu.Unlock()
	}
}

// cleanupLoop periodically removes expired mappings.
func (h *Hybrid1Session) cleanupLoop() {
	ticker := time.NewTicker(HashExpiryDuration / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.removeExpiredMappings()
		case <-h.recvStopChan:
			return
		}
	}
}

// removeExpiredMappings removes mappings that have exceeded HashExpiryDuration.
func (h *Hybrid1Session) removeExpiredMappings() {
	h.receiverMu.Lock()
	defer h.receiverMu.Unlock()

	now := time.Now()
	for hash, mapping := range h.receiverMap {
		mapping.mu.RLock()
		expired := now.Sub(mapping.LastRefresh) > HashExpiryDuration
		mapping.mu.RUnlock()

		if expired {
			delete(h.receiverMap, hash)
		}
	}
}

// Close closes the hybrid1 session and all associated resources.
func (h *Hybrid1Session) Close() error {
	h.closeMu.Lock()
	if h.closed {
		h.closeMu.Unlock()
		return nil
	}
	h.closed = true
	h.closeMu.Unlock()

	// Stop receive loop.
	close(h.recvStopChan)

	// Close datagram subsession.
	if h.datagramSub != nil {
		h.datagramSub.Close()
	}

	// Signal cleanup is done.
	close(h.cleanupDone)

	return nil
}

// ID returns the unique identifier for this session.
func (h *Hybrid1Session) ID() string {
	return h.id
}

// Addr returns the local I2P address for this session.
func (h *Hybrid1Session) Addr() i2pkeys.I2PAddr {
	return h.localAddr
}

// LocalPort returns the local port for this session.
func (h *Hybrid1Session) LocalPort() uint16 {
	return h.localPort
}

// IsClosed returns whether this session has been closed.
func (h *Hybrid1Session) IsClosed() bool {
	h.closeMu.RLock()
	defer h.closeMu.RUnlock()
	return h.closed
}

// DatagramSubSession returns the underlying datagram subsession.
func (h *Hybrid1Session) DatagramSubSession() *primary.DatagramSubSession {
	return h.datagramSub
}

// PacketConn returns a net.PacketConn interface for this session.
func (h *Hybrid1Session) PacketConn() *Hybrid1PacketConn {
	return &Hybrid1PacketConn{session: h}
}

// WaitForClose blocks until the session is fully closed.
func (h *Hybrid1Session) WaitForClose() {
	<-h.cleanupDone
}

// getSenderState retrieves or creates sender state for a destination.
func (h *Hybrid1Session) getSenderState(dest i2pkeys.I2PAddr) *SenderState {
	key := dest.Base64()

	// Fast path.
	h.senderMu.RLock()
	state, exists := h.senderStates[key]
	h.senderMu.RUnlock()

	if exists {
		return state
	}

	// Slow path.
	h.senderMu.Lock()
	defer h.senderMu.Unlock()

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

// SenderStateCount returns the number of destination states being tracked.
func (h *Hybrid1Session) SenderStateCount() int {
	h.senderMu.RLock()
	defer h.senderMu.RUnlock()
	return len(h.senderStates)
}

// ReceiverMappingCount returns the number of sender hash mappings.
func (h *Hybrid1Session) ReceiverMappingCount() int {
	h.receiverMu.RLock()
	defer h.receiverMu.RUnlock()
	return len(h.receiverMap)
}

// Mutex for session-level operations.
var sessionMu sync.Mutex
