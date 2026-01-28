package hybrid2

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/go-i2p/go-sam-go/common"
	"github.com/go-i2p/go-sam-go/datagram"
	"github.com/go-i2p/go-sam-go/datagram3"
	"github.com/go-i2p/go-sam-go/primary"
	"github.com/go-i2p/i2pkeys"
)

// ErrSessionClosed is returned when operations are attempted on a closed session.
var ErrSessionClosed = errors.New("hybrid2: session is closed")

// ErrSourceNotFound is returned when a datagram3 source hash cannot be resolved.
var ErrSourceNotFound = errors.New("hybrid2: source hash not found in mapping")

// SenderHash represents a 32-byte hash derived from a datagram source.
// This hash is used by datagram3 for sender identification and is resolved
// to a full destination address using mappings established by datagram2.
type SenderHash [32]byte

// SenderState tracks the send counter for a specific destination.
// The counter determines when to use datagram2 (authenticated) vs datagram3 (low-overhead).
type SenderState struct {
	// Destination is the I2P address this state tracks.
	Destination i2pkeys.I2PAddr

	// Counter is incremented on each send. When counter % RepliableInterval == 0,
	// datagram2 is used for authentication; otherwise datagram3 is used.
	Counter uint64

	// LastSendTime records when the last datagram was sent to this destination.
	LastSendTime time.Time

	mu sync.Mutex
}

// ReceiverMapping maps a sender hash to a full destination address.
// These mappings are established when datagram2 messages are received and
// used to resolve datagram3 source hashes.
type ReceiverMapping struct {
	// Hash is the 32-byte hash identifying this sender.
	Hash SenderHash

	// Destination is the full I2P destination address for this sender.
	Destination i2pkeys.I2PAddr

	// FirstSeen is when this mapping was first established.
	FirstSeen time.Time

	// LastRefresh is when this mapping was last updated (via datagram2).
	LastRefresh time.Time

	mu sync.RWMutex
}

// HybridDatagram represents a received datagram with resolved source information.
type HybridDatagram struct {
	// Data is the datagram payload.
	Data []byte

	// Source is the resolved full destination address of the sender.
	// For datagram2, this is the authenticated source.
	// For datagram3, this is resolved from the hash mapping.
	Source i2pkeys.I2PAddr

	// SourceHash is the 32-byte hash of the sender's address.
	SourceHash SenderHash

	// WasDatagram2 indicates whether this datagram was received via datagram2.
	// If true, the source is authenticated. If false, it was resolved from cache.
	WasDatagram2 bool

	// Timestamp is when this datagram was received.
	Timestamp time.Time
}

// HybridSession manages both datagram2 and datagram3 subsessions for efficient
// datagram routing. It automatically selects the appropriate protocol based on
// the send counter and maintains hash-to-destination mappings for receivers.
type HybridSession struct {
	// Primary session reference
	primary *primary.PrimarySession

	// SAM connection for session operations
	sam *common.SAM

	// Datagram subsessions
	datagram2Sub *primary.DatagramSubSession  // For authenticated sends (repliable)
	datagram3Sub *primary.Datagram3SubSession // For bulk sends (low overhead)

	// Sender state: tracks counter per destination
	senderStates map[string]*SenderState // key: destination base64
	senderMu     sync.RWMutex

	// Receiver state: maps hashes to destinations (managed by ReceiverState)
	receiverState *ReceiverState
	receiverMap   map[SenderHash]*ReceiverMapping // legacy field for backward compat
	receiverMu    sync.RWMutex

	// Datagram readers for receiving
	datagram2Reader *datagram.DatagramReader
	datagram3Reader *datagram3.Datagram3Reader

	// Receive channels for multiplexed reading
	recvChan     chan *HybridDatagram // unified receive channel
	recvErrChan  chan error           // error channel
	recvStopChan chan struct{}        // signal to stop receive loops

	// Legacy receive channels (for backward compat)
	receiveChannel chan *HybridDatagram
	errorChannel   chan error

	// Session metadata
	id        string
	localAddr i2pkeys.I2PAddr
	options   []string

	// Lifecycle management
	closed  bool
	closeMu sync.RWMutex

	// Cleanup control
	cleanupDone chan struct{}
}

// HybridPacketConn implements net.PacketConn using hybrid2 logic.
// This allows the hybrid session to be used with standard Go networking patterns.
type HybridPacketConn struct {
	session *HybridSession
}

// HybridAddr wraps an I2P address for the net.Addr interface.
type HybridAddr struct {
	i2pkeys.I2PAddr
}

// Network returns the network type for this address.
func (a *HybridAddr) Network() string {
	return "hybrid2"
}

// String returns the base32 representation of the address.
func (a *HybridAddr) String() string {
	return a.I2PAddr.Base32()
}

// HybridSender defines the interface for hybrid send operations.
type HybridSender interface {
	// SendDatagram sends data using the hybrid protocol.
	// Automatically chooses datagram2 or datagram3 based on counter.
	SendDatagram(data []byte, dest i2pkeys.I2PAddr) error

	// ForceDatagram2 forces a datagram2 send (identity refresh).
	ForceDatagram2(data []byte, dest i2pkeys.I2PAddr) error
}

// HybridReceiver defines the interface for hybrid receive operations.
type HybridReceiver interface {
	// ReceiveDatagram receives and processes a hybrid datagram.
	// Handles both datagram2 (updates mapping) and datagram3 (uses mapping).
	ReceiveDatagram() (*HybridDatagram, error)

	// LookupSender resolves a hash to a full destination address.
	LookupSender(hash SenderHash) (i2pkeys.I2PAddr, bool)
}

// Compile-time interface assertions - uncomment when implementations are complete:
var _ net.PacketConn = (*HybridPacketConn)(nil)
var _ HybridSender = (*HybridSession)(nil)
var _ HybridReceiver = (*HybridSession)(nil)
