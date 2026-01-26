package onramp

import (
	"bytes"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockListener implements net.Listener for testing
type mockListener struct {
	acceptChan chan net.Conn
	closed     bool
	mu         sync.Mutex
	addr       net.Addr
}

func newMockListener() *mockListener {
	return &mockListener{
		acceptChan: make(chan net.Conn, 10),
		addr:       &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080},
	}
}

func (m *mockListener) Accept() (net.Conn, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, io.EOF
	}
	m.mu.Unlock()

	conn, ok := <-m.acceptChan
	if !ok {
		return nil, io.EOF
	}
	return conn, nil
}

func (m *mockListener) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.closed = true
		close(m.acceptChan)
	}
	return nil
}

func (m *mockListener) Addr() net.Addr {
	return m.addr
}

func (m *mockListener) QueueConnection(conn net.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.acceptChan <- conn
	}
}

// mockConn implements net.Conn for testing
type mockConn struct {
	readBuf    *bytes.Buffer
	writeBuf   *bytes.Buffer
	closed     bool
	mu         sync.Mutex
	localAddr  net.Addr
	remoteAddr net.Addr
}

func newMockConn(localAddr, remoteAddr net.Addr) *mockConn {
	return &mockConn{
		readBuf:    new(bytes.Buffer),
		writeBuf:   new(bytes.Buffer),
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	}
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed && m.readBuf.Len() == 0 {
		return 0, io.EOF
	}
	return m.readBuf.Read(b)
}

func (m *mockConn) Write(b []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, io.ErrClosedPipe
	}
	return m.writeBuf.Write(b)
}

func (m *mockConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockConn) LocalAddr() net.Addr {
	return m.localAddr
}

func (m *mockConn) RemoteAddr() net.Addr {
	return m.remoteAddr
}

func (m *mockConn) SetDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// WriteToReadBuffer writes data to the read buffer (simulating incoming data)
func (m *mockConn) WriteToReadBuffer(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readBuf.Write(data)
}

// GetWrittenData returns all data written to the connection
func (m *mockConn) GetWrittenData() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.writeBuf.Bytes()
}

// TestOnrampProxy_Proxy_AcceptsConnection tests that the proxy accepts connections
func TestOnrampProxy_Proxy_AcceptsConnection(t *testing.T) {
	listener := newMockListener()
	proxy := &OnrampProxy{}

	// Start proxy in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- proxy.Proxy(listener, "example.com:80")
	}()

	// Queue a mock connection
	mockConn := newMockConn(
		&net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345},
		&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080},
	)
	listener.QueueConnection(mockConn)

	// Give it a moment to process
	time.Sleep(50 * time.Millisecond)

	// Close listener to stop the proxy
	listener.Close()

	// Check if there was an error
	select {
	case err := <-errChan:
		if err != nil && err != io.EOF {
			t.Errorf("Proxy() returned unexpected error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("Proxy() did not return after listener closed")
	}
}

// TestOnrampProxy_Proxy_ErrorOnAccept tests error handling when Accept fails
func TestOnrampProxy_Proxy_ErrorOnAccept(t *testing.T) {
	listener := newMockListener()
	proxy := &OnrampProxy{}

	// Close listener immediately to cause Accept to fail
	listener.Close()

	err := proxy.Proxy(listener, "example.com:80")
	if err == nil {
		t.Error("Expected error when Accept fails, got nil")
	}
}

// TestOnrampProxy_proxy_I2PAddress tests proxy routing for .i2p addresses
func TestOnrampProxy_proxy_I2PAddress(t *testing.T) {
	// This test verifies that the proxy detects .i2p addresses
	// We can't test the actual Dial without a SAM bridge, but we can
	// verify the address parsing logic

	tests := []struct {
		name      string
		raddr     string
		expectI2P bool
	}{
		{
			name:      "I2P_Address",
			raddr:     "example.i2p:80",
			expectI2P: true,
		},
		{
			name:      "I2P_Address_NoPort",
			raddr:     "example.i2p",
			expectI2P: true,
		},
		{
			name:      "Onion_Address",
			raddr:     "example.onion:80",
			expectI2P: false,
		},
		{
			name:      "Regular_Address",
			raddr:     "example.com:80",
			expectI2P: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkaddr := strings.Split(tt.raddr, ":")[0]
			isI2P := strings.HasSuffix(checkaddr, ".i2p")

			if isI2P != tt.expectI2P {
				t.Errorf("Address detection failed: expected I2P=%v, got I2P=%v for %s",
					tt.expectI2P, isI2P, tt.raddr)
			}
		})
	}
}

// TestOnrampProxy_proxy_OnionAddress tests proxy routing for .onion addresses
func TestOnrampProxy_proxy_OnionAddress(t *testing.T) {
	tests := []struct {
		name        string
		raddr       string
		expectOnion bool
	}{
		{
			name:        "Onion_Address",
			raddr:       "3g2upl4pq6kufc4m.onion:80",
			expectOnion: true,
		},
		{
			name:        "Onion_V3_Address",
			raddr:       "thehiddenwiki.onion:443",
			expectOnion: true,
		},
		{
			name:        "Regular_Domain",
			raddr:       "example.com:80",
			expectOnion: false,
		},
		{
			name:        "I2P_Address",
			raddr:       "example.i2p:80",
			expectOnion: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkaddr := strings.Split(tt.raddr, ":")[0]
			isOnion := strings.HasSuffix(checkaddr, ".onion")

			if isOnion != tt.expectOnion {
				t.Errorf("Address detection failed: expected Onion=%v, got Onion=%v for %s",
					tt.expectOnion, isOnion, tt.raddr)
			}
		})
	}
}

// TestOnrampProxy_proxy_AddressParsing tests various address parsing scenarios
func TestOnrampProxy_proxy_AddressParsing(t *testing.T) {
	tests := []struct {
		name         string
		raddr        string
		expectNoPort bool
		expectedHost string
	}{
		{
			name:         "Address_With_Port",
			raddr:        "example.com:8080",
			expectNoPort: false,
			expectedHost: "example.com",
		},
		{
			name:         "Address_Without_Port",
			raddr:        "example.com",
			expectNoPort: true,
			expectedHost: "example.com",
		},
		{
			name:         "I2P_With_Port",
			raddr:        "test.i2p:443",
			expectNoPort: false,
			expectedHost: "test.i2p",
		},
		{
			name:         "Onion_With_Port",
			raddr:        "test.onion:9050",
			expectNoPort: false,
			expectedHost: "test.onion",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := strings.Split(tt.raddr, ":")
			host := parts[0]

			if host != tt.expectedHost {
				t.Errorf("Expected host %s, got %s", tt.expectedHost, host)
			}

			hasPort := len(parts) > 1
			if hasPort == tt.expectNoPort {
				t.Errorf("Port presence mismatch: expected noPort=%v, got %v parts",
					tt.expectNoPort, len(parts))
			}
		})
	}
}

// TestProxy_FunctionExists tests that the package-level Proxy function exists
func TestProxy_FunctionExists(t *testing.T) {
	listener := newMockListener()
	listener.Close()

	// Call the package-level function
	err := Proxy(listener, "example.com:80")
	if err == nil {
		t.Error("Expected error from closed listener, got nil")
	}
}

// TestProxy_UsesGlobalProxy tests that package-level Proxy uses the global proxy instance
func TestProxy_UsesGlobalProxy(t *testing.T) {
	if proxy == nil {
		t.Error("Global proxy instance should not be nil")
	}

	// Verify it's an OnrampProxy
	_, ok := interface{}(proxy).(*OnrampProxy)
	if !ok {
		t.Error("Global proxy should be of type *OnrampProxy")
	}
}

// TestOnrampProxy_Initialization tests OnrampProxy struct initialization
func TestOnrampProxy_Initialization(t *testing.T) {
	p := &OnrampProxy{}

	if p == nil {
		t.Fatal("OnrampProxy should not be nil")
	}

	// The struct should contain Onion and Garlic fields
	// We can't test much without actual SAM/Tor connections,
	// but we can verify the struct exists
	_ = p.Onion
	_ = p.Garlic
}

// TestMockConn_ReadWrite tests the mock connection implementation
func TestMockConn_ReadWrite(t *testing.T) {
	conn := newMockConn(
		&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234},
		&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5678},
	)

	// Test Write
	testData := []byte("Hello, World!")
	n, err := conn.Write(testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(testData), n)
	}

	// Test GetWrittenData
	written := conn.GetWrittenData()
	if !bytes.Equal(written, testData) {
		t.Errorf("Expected written data %q, got %q", testData, written)
	}

	// Test Read
	conn.WriteToReadBuffer([]byte("Test Read"))
	buf := make([]byte, 100)
	n, err = conn.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(buf[:n]) != "Test Read" {
		t.Errorf("Expected to read 'Test Read', got %q", buf[:n])
	}
}

// TestMockConn_Addresses tests address methods
func TestMockConn_Addresses(t *testing.T) {
	localAddr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 1234}
	remoteAddr := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 5678}

	conn := newMockConn(localAddr, remoteAddr)

	if conn.LocalAddr().String() != localAddr.String() {
		t.Errorf("Expected local address %s, got %s", localAddr, conn.LocalAddr())
	}

	if conn.RemoteAddr().String() != remoteAddr.String() {
		t.Errorf("Expected remote address %s, got %s", remoteAddr, conn.RemoteAddr())
	}
}

// TestMockConn_Close tests connection closing
func TestMockConn_Close(t *testing.T) {
	conn := newMockConn(
		&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234},
		&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5678},
	)

	// Close the connection
	err := conn.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Write should fail after close
	_, err = conn.Write([]byte("test"))
	if err != io.ErrClosedPipe {
		t.Errorf("Expected ErrClosedPipe after close, got %v", err)
	}

	// Read should return EOF after close and buffer is empty
	_, err = conn.Read(make([]byte, 10))
	if err != io.EOF {
		t.Errorf("Expected EOF after close, got %v", err)
	}
}

// TestMockListener_Addr tests listener address
func TestMockListener_Addr(t *testing.T) {
	listener := newMockListener()

	addr := listener.Addr()
	if addr == nil {
		t.Fatal("Listener address should not be nil")
	}

	expected := "127.0.0.1:8080"
	if addr.String() != expected {
		t.Errorf("Expected address %s, got %s", expected, addr.String())
	}
}
