package hybrid2

import (
	"sync"
	"testing"
	"time"

	"github.com/go-i2p/i2pkeys"
)

// TestSenderHash verifies that SenderHash is 32 bytes.
func TestSenderHash(t *testing.T) {
	var hash SenderHash
	if len(hash) != 32 {
		t.Errorf("SenderHash should be 32 bytes, got %d", len(hash))
	}
}

// TestComputeSenderHash verifies hash computation from I2P addresses.
func TestComputeSenderHash(t *testing.T) {
	// Test with any address string - function should be deterministic
	testAddr := i2pkeys.I2PAddr("test-address")

	hash1 := ComputeSenderHash(testAddr)
	hash2 := ComputeSenderHash(testAddr)

	// Same address should produce same hash (deterministic)
	if hash1 != hash2 {
		t.Error("ComputeSenderHash should be deterministic")
	}

	// Verify hash is the correct size
	if len(hash1) != 32 {
		t.Errorf("Hash should be 32 bytes, got %d", len(hash1))
	}
}

// TestSenderState tests the sender state counter logic.
func TestSenderState(t *testing.T) {
	state := &SenderState{
		Destination: i2pkeys.I2PAddr("test"),
		Counter:     0,
	}

	// First message (counter=0) should use datagram2
	if !state.shouldUseDatagram2() {
		t.Error("Counter 0 should use datagram2")
	}

	// Increment through messages
	for i := uint64(0); i < 99; i++ {
		state.incrementCounter()
	}

	// Counter 99 should not use datagram2
	if state.shouldUseDatagram2() {
		t.Error("Counter 99 should not use datagram2")
	}

	// Counter 100 should use datagram2
	state.incrementCounter()
	if !state.shouldUseDatagram2() {
		t.Error("Counter 100 should use datagram2")
	}
}

// TestSenderStateThreadSafety verifies concurrent access to SenderState.
func TestSenderStateThreadSafety(t *testing.T) {
	state := &SenderState{
		Destination: i2pkeys.I2PAddr("test"),
		Counter:     0,
	}

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			state.shouldUseDatagram2()
			state.incrementCounter()
		}()
	}

	wg.Wait()

	// After 100 increments, counter should be 100
	if state.Counter != uint64(numGoroutines) {
		t.Errorf("Expected counter to be %d, got %d", numGoroutines, state.Counter)
	}
}

// TestReceiverState tests the receiver mapping functionality.
func TestReceiverState(t *testing.T) {
	rs := NewReceiverState()
	defer rs.Stop()

	// Create a test hash
	var hash SenderHash
	copy(hash[:], []byte("test-hash-1234567890123456"))

	dest := i2pkeys.I2PAddr("test-destination")

	// Register sender
	rs.RegisterSender(hash, dest)

	// Lookup should succeed
	foundDest, found := rs.LookupSender(hash)
	if !found {
		t.Error("LookupSender should find registered sender")
	}
	if foundDest != dest {
		t.Errorf("Expected destination %s, got %s", dest, foundDest)
	}

	// Verify count
	if rs.MappingCount() != 1 {
		t.Errorf("Expected 1 mapping, got %d", rs.MappingCount())
	}

	// Register same sender again (should refresh, not add new)
	rs.RegisterSender(hash, dest)
	if rs.MappingCount() != 1 {
		t.Errorf("Re-registering should not add new mapping, got %d", rs.MappingCount())
	}

	// Lookup unknown hash should fail
	var unknownHash SenderHash
	copy(unknownHash[:], []byte("unknown-hash-12345678901"))
	_, found = rs.LookupSender(unknownHash)
	if found {
		t.Error("LookupSender should not find unknown hash")
	}
}

// TestReceiverStateThreadSafety verifies concurrent access to ReceiverState.
func TestReceiverStateThreadSafety(t *testing.T) {
	rs := NewReceiverState()
	defer rs.Stop()

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			var hash SenderHash
			// Create unique hash for each goroutine
			copy(hash[:], []byte{byte(idx), byte(idx >> 8)})
			dest := i2pkeys.I2PAddr("dest-" + string(rune(idx)))

			rs.RegisterSender(hash, dest)
			rs.LookupSender(hash)
		}(i)
	}

	wg.Wait()

	// Should have registered multiple senders
	if rs.MappingCount() == 0 {
		t.Error("Expected at least some mappings after concurrent operations")
	}
}

// TestReceiverMappingExpiry tests that expired mappings are detected.
func TestReceiverMappingExpiry(t *testing.T) {
	entry := &ReceiverMappingEntry{
		Hash:        SenderHash{},
		Destination: i2pkeys.I2PAddr("test"),
		FirstSeen:   time.Now().Add(-2 * HashExpiryDuration),
		LastRefresh: time.Now().Add(-2 * HashExpiryDuration),
	}

	if !entry.isExpired() {
		t.Error("Mapping older than HashExpiryDuration should be expired")
	}

	// Refresh should update LastRefresh
	entry.refresh()
	if entry.isExpired() {
		t.Error("Recently refreshed mapping should not be expired")
	}
}

// TestHybridDatagram tests the HybridDatagram structure.
func TestHybridDatagram(t *testing.T) {
	dg := &HybridDatagram{
		Data:         []byte("test data"),
		Source:       i2pkeys.I2PAddr("source"),
		SourceHash:   SenderHash{1, 2, 3},
		WasDatagram2: true,
		Timestamp:    time.Now(),
	}

	if string(dg.Data) != "test data" {
		t.Error("HybridDatagram Data field mismatch")
	}
	if !dg.WasDatagram2 {
		t.Error("HybridDatagram WasDatagram2 should be true")
	}
}

// TestConstants verifies the protocol constants are set correctly.
func TestConstants(t *testing.T) {
	if RepliableInterval != 100 {
		t.Errorf("RepliableInterval should be 100, got %d", RepliableInterval)
	}
	if FirstMessageIndex != 0 {
		t.Errorf("FirstMessageIndex should be 0, got %d", FirstMessageIndex)
	}
	if HashExpiryDuration != 10*time.Minute {
		t.Errorf("HashExpiryDuration should be 10 minutes, got %v", HashExpiryDuration)
	}
	if RecvChanBufferSize != DefaultReceiveBufferSize {
		t.Errorf("RecvChanBufferSize should equal DefaultReceiveBufferSize")
	}
}

// TestHybridAddr tests the HybridAddr net.Addr implementation.
func TestHybridAddr(t *testing.T) {
	addr := &HybridAddr{i2pkeys.I2PAddr("test")}

	if addr.Network() != "hybrid2" {
		t.Errorf("Expected network 'hybrid2', got '%s'", addr.Network())
	}

	// String should return base32 representation
	str := addr.String()
	if str == "" {
		t.Error("HybridAddr.String() should not be empty")
	}
}

// TestErrors tests the error variables are properly defined.
func TestErrors(t *testing.T) {
	if ErrSessionClosed == nil {
		t.Error("ErrSessionClosed should not be nil")
	}
	if ErrSourceNotFound == nil {
		t.Error("ErrSourceNotFound should not be nil")
	}
	if ErrTimeout == nil {
		t.Error("ErrTimeout should not be nil")
	}
}

// TestHybridPacketConnDeadlines tests deadline setting and retrieval.
func TestHybridPacketConnDeadlines(t *testing.T) {
	// Create a minimal HybridPacketConn for deadline testing
	// Note: We can't fully test WriteTo without a real session,
	// but we can test the deadline storage and retrieval logic.
	conn := &HybridPacketConn{}

	// Test SetReadDeadline
	readDeadline := time.Now().Add(10 * time.Second)
	if err := conn.SetReadDeadline(readDeadline); err != nil {
		t.Errorf("SetReadDeadline returned error: %v", err)
	}
	conn.deadlineMu.RLock()
	if !conn.readDeadline.Equal(readDeadline) {
		t.Error("Read deadline was not set correctly")
	}
	conn.deadlineMu.RUnlock()

	// Test SetWriteDeadline
	writeDeadline := time.Now().Add(20 * time.Second)
	if err := conn.SetWriteDeadline(writeDeadline); err != nil {
		t.Errorf("SetWriteDeadline returned error: %v", err)
	}
	conn.deadlineMu.RLock()
	if !conn.writeDeadline.Equal(writeDeadline) {
		t.Error("Write deadline was not set correctly")
	}
	conn.deadlineMu.RUnlock()

	// Test SetDeadline (sets both)
	bothDeadline := time.Now().Add(30 * time.Second)
	if err := conn.SetDeadline(bothDeadline); err != nil {
		t.Errorf("SetDeadline returned error: %v", err)
	}
	conn.deadlineMu.RLock()
	if !conn.readDeadline.Equal(bothDeadline) {
		t.Error("SetDeadline did not set read deadline")
	}
	if !conn.writeDeadline.Equal(bothDeadline) {
		t.Error("SetDeadline did not set write deadline")
	}
	conn.deadlineMu.RUnlock()

	// Test zero deadline (clears deadline)
	if err := conn.SetDeadline(time.Time{}); err != nil {
		t.Errorf("SetDeadline with zero time returned error: %v", err)
	}
	conn.deadlineMu.RLock()
	if !conn.readDeadline.IsZero() {
		t.Error("Zero deadline should clear read deadline")
	}
	if !conn.writeDeadline.IsZero() {
		t.Error("Zero deadline should clear write deadline")
	}
	conn.deadlineMu.RUnlock()
}

// TestHybridPacketConnClose tests that closing a PacketConn only closes the wrapper.
func TestHybridPacketConnClose(t *testing.T) {
	// Create a minimal PacketConn for close testing
	conn := &HybridPacketConn{}

	// Verify not closed initially
	if conn.isClosed() {
		t.Error("PacketConn should not be closed initially")
	}

	// Close the connection
	if err := conn.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Verify closed after Close()
	if !conn.isClosed() {
		t.Error("PacketConn should be closed after Close()")
	}

	// Calling Close again should not error
	if err := conn.Close(); err != nil {
		t.Errorf("Second Close returned error: %v", err)
	}
}

// TestHybridPacketConnCloseDoesNotAffectOther verifies that closing one PacketConn
// does not affect other PacketConns from the same session.
func TestHybridPacketConnCloseDoesNotAffectOther(t *testing.T) {
	// Create two PacketConns (simulating multiple wrappers of same session)
	// Note: In real use, these would share a HybridSession, but for this test
	// we just need to verify the wrapper closure logic is independent.
	conn1 := &HybridPacketConn{}
	conn2 := &HybridPacketConn{}

	// Close conn1
	if err := conn1.Close(); err != nil {
		t.Errorf("Close conn1 returned error: %v", err)
	}

	// Verify conn1 is closed but conn2 is not
	if !conn1.isClosed() {
		t.Error("conn1 should be closed")
	}
	if conn2.isClosed() {
		t.Error("conn2 should NOT be closed when conn1 is closed")
	}
}

// TestErrPacketConnClosed verifies the error variable is properly defined.
func TestErrPacketConnClosed(t *testing.T) {
	if ErrPacketConnClosed == nil {
		t.Error("ErrPacketConnClosed should not be nil")
	}
	if ErrPacketConnClosed.Error() != "hybrid2: packet connection is closed" {
		t.Errorf("Unexpected error message: %s", ErrPacketConnClosed.Error())
	}
}

// BenchmarkComputeSenderHash benchmarks hash computation.
func BenchmarkComputeSenderHash(b *testing.B) {
	addr := i2pkeys.I2PAddr("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ComputeSenderHash(addr)
	}
}

// BenchmarkReceiverStateLookup benchmarks hash lookups.
func BenchmarkReceiverStateLookup(b *testing.B) {
	rs := NewReceiverState()
	defer rs.Stop()

	// Pre-populate with some mappings
	for i := 0; i < 1000; i++ {
		var hash SenderHash
		copy(hash[:], []byte{byte(i), byte(i >> 8)})
		rs.RegisterSender(hash, i2pkeys.I2PAddr("dest"))
	}

	var lookupHash SenderHash
	copy(lookupHash[:], []byte{50, 0})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rs.LookupSender(lookupHash)
	}
}

// BenchmarkSenderStateIncrement benchmarks counter increments.
func BenchmarkSenderStateIncrement(b *testing.B) {
	state := &SenderState{
		Destination: i2pkeys.I2PAddr("test"),
		Counter:     0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state.incrementCounter()
	}
}
