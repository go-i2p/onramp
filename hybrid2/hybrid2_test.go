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
// RepliableInterval is 500, so datagram2 is used at counter 0, 500, 1000, etc.
func TestSenderState(t *testing.T) {
	state := &SenderState{
		Destination:      i2pkeys.I2PAddr("test"),
		Counter:          0,
		UnackedDatagrams: make(map[uint64]time.Time),
	}

	// First message (counter=0) should use datagram2
	if !state.shouldUseDatagram2() {
		t.Error("Counter 0 should use datagram2")
	}

	// Increment to counter 1 and verify datagram3 is used
	state.incrementCounter()
	if state.shouldUseDatagram2() {
		t.Error("Counter 1 should not use datagram2")
	}

	// Increment to counter 499 (still not at interval)
	for i := uint64(1); i < 499; i++ {
		state.incrementCounter()
	}
	// Counter is now 499, should not use datagram2
	if state.shouldUseDatagram2() {
		t.Error("Counter 499 should not use datagram2")
	}

	// Counter 500 should use datagram2 (500 % 500 == 0)
	state.incrementCounter()
	if !state.shouldUseDatagram2() {
		t.Error("Counter 500 should use datagram2")
	}

	// Counter 501 should not use datagram2
	state.incrementCounter()
	if state.shouldUseDatagram2() {
		t.Error("Counter 501 should not use datagram2")
	}
}

// TestSenderStateThreadSafety verifies concurrent access to SenderState.
func TestSenderStateThreadSafety(t *testing.T) {
	state := &SenderState{
		Destination:      i2pkeys.I2PAddr("test"),
		Counter:          0,
		UnackedDatagrams: make(map[uint64]time.Time),
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
// Note: RepliableInterval was changed from 100 to 500 for better efficiency.
// This test validates the current configuration values.
func TestConstants(t *testing.T) {
	if RepliableInterval != 500 {
		t.Errorf("RepliableInterval should be 500, got %d", RepliableInterval)
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
		Destination:      i2pkeys.I2PAddr("test"),
		Counter:          0,
		UnackedDatagrams: make(map[uint64]time.Time),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state.incrementCounter()
	}
}

// TestTimeBasedTrigger verifies that time-based triggering works correctly.
func TestTimeBasedTrigger(t *testing.T) {
	state := &SenderState{
		Destination:       i2pkeys.I2PAddr("test"),
		Counter:           50,                               // Not at interval boundary
		LastDatagram2Time: time.Now().Add(-5 * time.Minute), // 5 minutes ago
		UnackedDatagrams:  make(map[uint64]time.Time),
	}

	// Should trigger datagram2 due to time elapsed (> 4 minutes)
	if !state.shouldUseDatagram2() {
		t.Error("Should use datagram2 after RepliableTimeInterval elapsed")
	}

	// Reset to recent time
	state.LastDatagram2Time = time.Now()

	// Should not trigger datagram2 (counter not at boundary, time recent)
	if state.shouldUseDatagram2() {
		t.Error("Should not use datagram2 when time recent and counter not at boundary")
	}
}

// TestZeroLastDatagram2Time verifies that zero time forces datagram2.
func TestZeroLastDatagram2Time(t *testing.T) {
	state := &SenderState{
		Destination:       i2pkeys.I2PAddr("test"),
		Counter:           50,
		LastDatagram2Time: time.Time{}, // Zero value
		UnackedDatagrams:  make(map[uint64]time.Time),
	}

	// Zero time should force datagram2
	if !state.shouldUseDatagram2() {
		t.Error("Zero LastDatagram2Time should force datagram2")
	}
}

// TestACKMessageEncoding tests ACK request/response encoding and decoding.
func TestACKMessageEncoding(t *testing.T) {
	// Test ACK request encoding
	seqNum := uint64(12345)
	originalData := []byte("test payload data")

	encoded := encodeAckRequest(seqNum, originalData)

	// Decode and verify
	isReq, isResp, decodedSeq, data := decodeAckMessage(encoded)

	if !isReq {
		t.Error("Should be identified as ACK request")
	}
	if isResp {
		t.Error("Should not be identified as ACK response")
	}
	if decodedSeq != seqNum {
		t.Errorf("Sequence number mismatch: expected %d, got %d", seqNum, decodedSeq)
	}
	if string(data) != string(originalData) {
		t.Errorf("Data mismatch: expected %s, got %s", originalData, data)
	}

	// Test ACK response encoding
	ackedSeqNum := uint64(67890)
	ackResponse := encodeAckResponse(ackedSeqNum)

	isReq, isResp, decodedSeq, data = decodeAckMessage(ackResponse)

	if isReq {
		t.Error("Should not be identified as ACK request")
	}
	if !isResp {
		t.Error("Should be identified as ACK response")
	}
	if decodedSeq != ackedSeqNum {
		t.Errorf("ACK sequence number mismatch: expected %d, got %d", ackedSeqNum, decodedSeq)
	}
	if len(data) != 0 {
		t.Error("ACK response should have no data payload")
	}

	// Test regular data (no ACK markers)
	regularData := []byte("regular data without markers")
	isReq, isResp, _, decoded := decodeAckMessage(regularData)

	if isReq || isResp {
		t.Error("Regular data should not be identified as ACK message")
	}
	if string(decoded) != string(regularData) {
		t.Error("Regular data should pass through unchanged")
	}
}

// TestProcessAck verifies RTT calculation and PathQuality updates.
func TestProcessAck(t *testing.T) {
	state := &SenderState{
		Destination:      i2pkeys.I2PAddr("test"),
		UnackedDatagrams: make(map[uint64]time.Time),
	}

	// Track a datagram
	seqNum := uint64(1)
	sendTime := time.Now().Add(-100 * time.Millisecond)
	state.UnackedDatagrams[seqNum] = sendTime

	// Process ACK
	state.processAck(seqNum)

	// Verify RTT was calculated (should be ~100ms)
	if state.RTT < 90 || state.RTT > 150 {
		t.Errorf("RTT calculation incorrect: got %d ms, expected ~100ms", state.RTT)
	}

	// Verify PathQuality improved
	if state.PathQuality <= 0.0 {
		t.Error("PathQuality should be positive after successful ACK")
	}

	// Verify unacked map was cleaned
	if len(state.UnackedDatagrams) != 0 {
		t.Error("UnackedDatagrams should be empty after ACK")
	}

	// Verify LastAckedSeqNum was updated
	if state.LastAckedSeqNum != seqNum {
		t.Errorf("LastAckedSeqNum should be %d, got %d", seqNum, state.LastAckedSeqNum)
	}
}

// TestCleanupStaleUnacked verifies timeout-based cleanup.
func TestCleanupStaleUnacked(t *testing.T) {
	state := &SenderState{
		Destination:      i2pkeys.I2PAddr("test"),
		UnackedDatagrams: make(map[uint64]time.Time),
		PathQuality:      1.0, // Start with perfect quality
	}

	// Add stale unacked message (older than AckTimeout)
	staleSeqNum := uint64(1)
	state.UnackedDatagrams[staleSeqNum] = time.Now().Add(-AckTimeout - time.Second)

	// Add recent unacked message
	recentSeqNum := uint64(2)
	state.UnackedDatagrams[recentSeqNum] = time.Now()

	// Run cleanup
	timedOut := state.cleanupStaleUnacked()

	// Verify stale message was removed
	if timedOut != 1 {
		t.Errorf("Expected 1 timeout, got %d", timedOut)
	}

	// Verify recent message remains
	if len(state.UnackedDatagrams) != 1 {
		t.Errorf("Expected 1 unacked message remaining, got %d", len(state.UnackedDatagrams))
	}

	if _, exists := state.UnackedDatagrams[recentSeqNum]; !exists {
		t.Error("Recent unacked message should not be removed")
	}

	// Verify PathQuality degraded
	if state.PathQuality >= 1.0 {
		t.Error("PathQuality should degrade after timeout")
	}
}

// TestPathStatsAPI verifies the path monitoring API.
func TestPathStatsAPI(t *testing.T) {
	// Create a minimal hybrid session structure for testing
	session := &HybridSession{
		senderStates: make(map[string]*SenderState),
	}

	// Add test sender state
	dest := i2pkeys.I2PAddr("test-destination")
	state := &SenderState{
		Destination:         dest,
		Counter:             100,
		LastDatagram2Time:   time.Now().Add(-30 * time.Second),
		LastDatagram2SeqNum: 10,
		LastAckedSeqNum:     8,
		RTT:                 150,
		PathQuality:         0.95,
		UnackedDatagrams:    make(map[uint64]time.Time),
	}
	state.UnackedDatagrams[9] = time.Now().Add(-2 * time.Second)
	state.UnackedDatagrams[10] = time.Now().Add(-1 * time.Second)

	session.senderStates[dest.Base64()] = state

	// Get stats
	stats := session.GetPathStats(dest)

	if stats == nil {
		t.Fatal("GetPathStats returned nil")
	}

	if stats.RTT != 150 {
		t.Errorf("RTT mismatch: expected 150, got %d", stats.RTT)
	}

	if stats.PathQuality != 0.95 {
		t.Errorf("PathQuality mismatch: expected 0.95, got %f", stats.PathQuality)
	}

	if stats.UnackedCount != 2 {
		t.Errorf("UnackedCount mismatch: expected 2, got %d", stats.UnackedCount)
	}

	if stats.TotalSent != 10 {
		t.Errorf("TotalSent mismatch: expected 10, got %d", stats.TotalSent)
	}

	if stats.TotalAcked != 8 {
		t.Errorf("TotalAcked mismatch: expected 8, got %d", stats.TotalAcked)
	}

	// Test GetAllPathStats
	allStats := session.GetAllPathStats()
	if len(allStats) != 1 {
		t.Errorf("Expected 1 path stat, got %d", len(allStats))
	}
}

// TestRTTExponentialMovingAverage verifies RTT calculation.
func TestRTTExponentialMovingAverage(t *testing.T) {
	state := &SenderState{
		Destination:      i2pkeys.I2PAddr("test"),
		UnackedDatagrams: make(map[uint64]time.Time),
		RTT:              0,
	}

	// First RTT measurement
	state.UnackedDatagrams[1] = time.Now().Add(-100 * time.Millisecond)
	state.processAck(1)

	firstRTT := state.RTT
	if firstRTT < 90 || firstRTT > 110 {
		t.Errorf("First RTT should be ~100ms, got %d", firstRTT)
	}

	// Second RTT measurement (200ms) - should use EMA: (7*100 + 200)/8 = 112.5
	state.UnackedDatagrams[2] = time.Now().Add(-200 * time.Millisecond)
	state.processAck(2)

	secondRTT := state.RTT
	if secondRTT < 100 || secondRTT > 130 {
		t.Errorf("Second RTT should be ~112ms (EMA), got %d", secondRTT)
	}

	// Verify RTT increased (influenced by second measurement)
	if secondRTT <= firstRTT {
		t.Error("RTT should increase after higher latency measurement")
	}
}

// TestConstantsUpdated verifies new constants are set correctly.
func TestConstantsUpdated(t *testing.T) {
	if RepliableInterval != 500 {
		t.Errorf("RepliableInterval should be 500, got %d", RepliableInterval)
	}

	if RepliableTimeInterval != 4*time.Minute {
		t.Errorf("RepliableTimeInterval should be 4 minutes, got %v", RepliableTimeInterval)
	}

	if HashExpiryDuration != 10*time.Minute {
		t.Errorf("HashExpiryDuration should be 10 minutes, got %v", HashExpiryDuration)
	}

	if AckTimeout != 10*time.Second {
		t.Errorf("AckTimeout should be 10 seconds, got %v", AckTimeout)
	}

	if MaxUnackedDatagrams != 3 {
		t.Errorf("MaxUnackedDatagrams should be 3, got %d", MaxUnackedDatagrams)
	}

	if AckRequestInterval != 1 {
		t.Errorf("AckRequestInterval should be 1, got %d", AckRequestInterval)
	}
}
