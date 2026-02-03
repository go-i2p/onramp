package hybrid1

import (
	"testing"
	"time"

	"github.com/go-i2p/i2pkeys"
)

// TestConstants verifies that constants are correctly defined.
func TestConstants(t *testing.T) {
	// Test mode constants
	if HYBRID_MODE_V1 != 1 {
		t.Errorf("HYBRID_MODE_V1 = %d, want 1", HYBRID_MODE_V1)
	}
	if HYBRID_MODE_V2 != 2 {
		t.Errorf("HYBRID_MODE_V2 = %d, want 2", HYBRID_MODE_V2)
	}

	// Test MTU constant
	if I2P_UDP_MAX_MTU != 64*1024 {
		t.Errorf("I2P_UDP_MAX_MTU = %d, want %d", I2P_UDP_MAX_MTU, 64*1024)
	}

	// Test header size
	if I2PD_HEADER_SIZE != 4 {
		t.Errorf("I2PD_HEADER_SIZE = %d, want 4", I2PD_HEADER_SIZE)
	}

	// Test max payload size
	expectedMaxPayload := I2P_UDP_MAX_MTU - I2PD_HEADER_SIZE
	if I2PD_MAX_PAYLOAD_SIZE != expectedMaxPayload {
		t.Errorf("I2PD_MAX_PAYLOAD_SIZE = %d, want %d", I2PD_MAX_PAYLOAD_SIZE, expectedMaxPayload)
	}
}

// TestHybrid1AddrInterface verifies Hybrid1Addr implements net.Addr.
func TestHybrid1AddrInterface(t *testing.T) {
	addr := &Hybrid1Addr{
		I2PAddr: "test.b32.i2p",
		Port:    8080,
	}

	if addr.Network() != "hybrid1" {
		t.Errorf("Network() = %q, want %q", addr.Network(), "hybrid1")
	}

	if addr.String() == "" {
		t.Error("String() should not be empty")
	}
}

// TestSenderState tests sender state counter logic.
func TestSenderState(t *testing.T) {
	state := &SenderState{
		Counter: 0,
	}

	// Increment counter
	state.mu.Lock()
	state.Counter++
	state.LastSendTime = time.Now()
	state.mu.Unlock()

	if state.Counter != 1 {
		t.Errorf("Counter = %d, want 1", state.Counter)
	}

	if state.LastSendTime.IsZero() {
		t.Error("LastSendTime should not be zero")
	}
}

// TestReceiverMapping tests receiver mapping expiry.
func TestReceiverMapping(t *testing.T) {
	mapping := &ReceiverMapping{
		Destination: "test.b32.i2p",
		Port:        8080,
		FirstSeen:   time.Now(),
		LastRefresh: time.Now(),
	}

	// Mapping should not be expired immediately
	if time.Since(mapping.LastRefresh) > HashExpiryDuration {
		t.Error("Mapping should not be expired immediately")
	}
}

// TestComputeSenderHash tests hash computation.
func TestComputeSenderHash(t *testing.T) {
	addr := "test.b32.i2p"
	hash := computeSenderHash(i2pkeys.I2PAddr(addr))

	// Hash should not be all zeros
	allZero := true
	for _, b := range hash {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("Hash should not be all zeros")
	}

	// Same address should produce same hash
	hash2 := computeSenderHash(i2pkeys.I2PAddr(addr))
	if hash != hash2 {
		t.Error("Same address should produce same hash")
	}
}

// TestBuildI2PDFrame tests i2pd frame construction.
func TestBuildI2PDFrame(t *testing.T) {
	conn := &Hybrid1PacketConn{
		session: &Hybrid1Session{
			localPort: 1234,
		},
	}

	payload := []byte("test payload")
	srcPort := uint16(1234)
	dstPort := uint16(5678)

	frame := conn.buildI2PDFrame(payload, srcPort, dstPort)

	// Check frame length
	expectedLen := I2PD_HEADER_SIZE + len(payload)
	if len(frame) != expectedLen {
		t.Errorf("Frame length = %d, want %d", len(frame), expectedLen)
	}

	// Check source port (big-endian)
	gotSrcPort := uint16(frame[0])<<8 | uint16(frame[1])
	if gotSrcPort != srcPort {
		t.Errorf("Source port = %d, want %d", gotSrcPort, srcPort)
	}

	// Check destination port (big-endian)
	gotDstPort := uint16(frame[2])<<8 | uint16(frame[3])
	if gotDstPort != dstPort {
		t.Errorf("Destination port = %d, want %d", gotDstPort, dstPort)
	}

	// Check payload
	gotPayload := frame[I2PD_HEADER_SIZE:]
	if string(gotPayload) != string(payload) {
		t.Errorf("Payload = %q, want %q", string(gotPayload), string(payload))
	}
}

// TestHybrid1PacketConnClosed tests closed state handling.
func TestHybrid1PacketConnClosed(t *testing.T) {
	session := &Hybrid1Session{
		closed: false,
	}
	conn := &Hybrid1PacketConn{
		session: session,
		closed:  false,
	}

	// Initially not closed
	if conn.isClosed() {
		t.Error("PacketConn should not be closed initially")
	}

	// Close the connection
	if err := conn.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Should be closed now
	if !conn.isClosed() {
		t.Error("PacketConn should be closed after Close()")
	}
}

// TestHybrid1PacketConnDeadlines tests deadline setting.
func TestHybrid1PacketConnDeadlines(t *testing.T) {
	session := &Hybrid1Session{
		closed: false,
	}
	conn := &Hybrid1PacketConn{
		session: session,
	}

	deadline := time.Now().Add(10 * time.Second)

	// Test SetDeadline
	if err := conn.SetDeadline(deadline); err != nil {
		t.Errorf("SetDeadline() error = %v", err)
	}

	conn.deadlineMu.RLock()
	if !conn.readDeadline.Equal(deadline) {
		t.Error("Read deadline not set correctly by SetDeadline")
	}
	if !conn.writeDeadline.Equal(deadline) {
		t.Error("Write deadline not set correctly by SetDeadline")
	}
	conn.deadlineMu.RUnlock()

	// Test SetReadDeadline
	readDeadline := time.Now().Add(5 * time.Second)
	if err := conn.SetReadDeadline(readDeadline); err != nil {
		t.Errorf("SetReadDeadline() error = %v", err)
	}

	conn.deadlineMu.RLock()
	if !conn.readDeadline.Equal(readDeadline) {
		t.Error("Read deadline not set correctly by SetReadDeadline")
	}
	conn.deadlineMu.RUnlock()

	// Test SetWriteDeadline
	writeDeadline := time.Now().Add(15 * time.Second)
	if err := conn.SetWriteDeadline(writeDeadline); err != nil {
		t.Errorf("SetWriteDeadline() error = %v", err)
	}

	conn.deadlineMu.RLock()
	if !conn.writeDeadline.Equal(writeDeadline) {
		t.Error("Write deadline not set correctly by SetWriteDeadline")
	}
	conn.deadlineMu.RUnlock()
}

// TestHybrid1PacketConnLocalAddr tests LocalAddr implementation.
func TestHybrid1PacketConnLocalAddr(t *testing.T) {
	session := &Hybrid1Session{
		localAddr: "test.b32.i2p",
		localPort: 8080,
	}
	conn := &Hybrid1PacketConn{
		session: session,
	}

	addr := conn.LocalAddr()
	if addr == nil {
		t.Fatal("LocalAddr() returned nil")
	}

	if addr.Network() != "hybrid1" {
		t.Errorf("Network() = %q, want %q", addr.Network(), "hybrid1")
	}

	hybrid1Addr, ok := addr.(*Hybrid1Addr)
	if !ok {
		t.Fatal("LocalAddr() did not return *Hybrid1Addr")
	}

	if hybrid1Addr.Port != 8080 {
		t.Errorf("Port = %d, want 8080", hybrid1Addr.Port)
	}
}

// TestPayloadTooLarge tests payload size validation.
func TestPayloadTooLarge(t *testing.T) {
	session := &Hybrid1Session{
		closed:       false,
		localAddr:    "test.b32.i2p",
		senderStates: make(map[string]*SenderState),
	}
	conn := &Hybrid1PacketConn{
		session: session,
	}

	// Create a payload that's too large
	largePayload := make([]byte, I2PD_MAX_PAYLOAD_SIZE+1)

	addr := &Hybrid1Addr{
		I2PAddr: "dest.b32.i2p",
		Port:    8080,
	}

	_, err := conn.WriteTo(largePayload, addr)
	if err != ErrPayloadTooLarge {
		t.Errorf("WriteTo() error = %v, want %v", err, ErrPayloadTooLarge)
	}
}
