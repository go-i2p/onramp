package hybrid1

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/go-i2p/go-sam-go/primary"
	"github.com/go-i2p/i2pkeys"
)

// Common errors for hybrid1 operations.
var (
	// ErrSessionClosed is returned when operations are attempted on a closed session.
	ErrSessionClosed = errors.New("hybrid1: session is closed")

	// ErrTimeout is returned when a read or write operation exceeds its deadline.
	ErrTimeout = errors.New("hybrid1: operation timed out")

	// ErrInvalidAddress is returned when an address cannot be parsed.
	ErrInvalidAddress = errors.New("hybrid1: invalid address format")

	// ErrPayloadTooLarge is returned when payload exceeds maximum size.
	ErrPayloadTooLarge = errors.New("hybrid1: payload exceeds maximum size")
)

// Hybrid1Addr wraps an I2P address with port information for i2pd-style addressing.
type Hybrid1Addr struct {
	// I2PAddr is the underlying I2P destination address.
	i2pkeys.I2PAddr

	// Port is the i2pd-style port number (0-65535).
	Port uint16
}

// Network returns the network type for this address.
func (a *Hybrid1Addr) Network() string {
	return "hybrid1"
}

// String returns a string representation of the address.
func (a *Hybrid1Addr) String() string {
	return a.I2PAddr.Base32()
}

// SenderHash represents a 32-byte hash derived from a datagram source.
type SenderHash [32]byte

// SenderState tracks send state for a specific destination.
type SenderState struct {
	// Destination is the I2P address this state tracks.
	Destination i2pkeys.I2PAddr

	// Counter is incremented on each send.
	Counter uint64

	// LastSendTime records when the last datagram was sent.
	LastSendTime time.Time

	// LastRepliableTime tracks when the last repliable datagram was sent.
	LastRepliableTime time.Time

	// LocalPort is the local port for this session.
	LocalPort uint16

	// RemotePort is the remote port for this destination.
	RemotePort uint16

	mu sync.Mutex
}

// ReceiverMapping maps a sender hash to a full destination address.
type ReceiverMapping struct {
	// Hash is the 32-byte hash identifying this sender.
	Hash SenderHash

	// Destination is the full I2P destination address.
	Destination i2pkeys.I2PAddr

	// Port is the sender's port.
	Port uint16

	// FirstSeen is when this mapping was first established.
	FirstSeen time.Time

	// LastRefresh is when this mapping was last updated.
	LastRefresh time.Time

	mu sync.RWMutex
}

// Hybrid1Session manages i2pd-compatible datagram operations.
type Hybrid1Session struct {
	// Primary session for tunnel sharing.
	primary *primary.PrimarySession

	// Datagram subsession for actual communication.
	datagramSub *primary.DatagramSubSession

	// Sender states per destination.
	senderStates map[string]*SenderState
	senderMu     sync.RWMutex

	// Receiver mappings.
	receiverMap map[SenderHash]*ReceiverMapping
	receiverMu  sync.RWMutex

	// Receive channel for incoming datagrams.
	recvChan     chan *Hybrid1Datagram
	recvErrChan  chan error
	recvStopChan chan struct{}

	// Session metadata.
	id        string
	localAddr i2pkeys.I2PAddr
	localPort uint16
	options   []string

	// Lifecycle management.
	closed  bool
	closeMu sync.RWMutex

	// Cleanup control.
	cleanupDone chan struct{}
}

// Hybrid1Datagram represents a received datagram with i2pd framing.
type Hybrid1Datagram struct {
	// Data is the datagram payload (without i2pd header).
	Data []byte

	// Source is the resolved full destination address.
	Source i2pkeys.I2PAddr

	// SourcePort is the sender's port from the i2pd header.
	SourcePort uint16

	// DestPort is the destination port from the i2pd header.
	DestPort uint16

	// Timestamp is when this datagram was received.
	Timestamp time.Time
}

// Hybrid1PacketConn implements net.PacketConn with i2pd protocol compatibility.
// It wraps a SAM3 DatagramSession and handles i2pd-style datagram framing.
type Hybrid1PacketConn struct {
	session *Hybrid1Session

	// Deadline support.
	readDeadline  time.Time
	writeDeadline time.Time
	deadlineMu    sync.RWMutex

	// Closed flag for this wrapper.
	closed   bool
	closedMu sync.RWMutex
}

// Compile-time interface assertion.
var _ net.PacketConn = (*Hybrid1PacketConn)(nil)
