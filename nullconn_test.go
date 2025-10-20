//go:build !gen
// +build !gen

package onramp

import (
	"net"
	"testing"
	"time"
)

// TestNullConn_Read verifies that Read operation returns zero bytes and no error
func TestNullConn_Read(t *testing.T) {
	nc := &NullConn{}

	tests := []struct {
		name       string
		bufferSize int
	}{
		{
			name:       "SmallBuffer",
			bufferSize: 10,
		},
		{
			name:       "LargeBuffer",
			bufferSize: 1024,
		},
		{
			name:       "ZeroBuffer",
			bufferSize: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer := make([]byte, tt.bufferSize)
			n, err := nc.Read(buffer)

			if err != nil {
				t.Errorf("Read() error = %v, want nil", err)
			}

			if n != 0 {
				t.Errorf("Read() returned %d bytes, want 0", n)
			}
		})
	}
}

// TestNullConn_Write verifies that Write operation returns zero bytes and no error
func TestNullConn_Write(t *testing.T) {
	nc := &NullConn{}

	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "EmptyData",
			data: []byte{},
		},
		{
			name: "SmallData",
			data: []byte("hello"),
		},
		{
			name: "LargeData",
			data: make([]byte, 1024),
		},
		{
			name: "NilData",
			data: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := nc.Write(tt.data)

			if err != nil {
				t.Errorf("Write() error = %v, want nil", err)
			}

			if n != 0 {
				t.Errorf("Write() returned %d bytes, want 0", n)
			}
		})
	}
}

// TestNullConn_Close verifies that Close operation returns no error
func TestNullConn_Close_ReturnsNoError(t *testing.T) {
	nc := &NullConn{}

	err := nc.Close()

	if err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

// TestNullConn_Close_MultipleCallsSucceed verifies that multiple Close calls don't fail
func TestNullConn_Close_MultipleCallsSucceed(t *testing.T) {
	nc := &NullConn{}

	// First close
	if err := nc.Close(); err != nil {
		t.Errorf("First Close() error = %v, want nil", err)
	}

	// Second close
	if err := nc.Close(); err != nil {
		t.Errorf("Second Close() error = %v, want nil", err)
	}

	// Third close
	if err := nc.Close(); err != nil {
		t.Errorf("Third Close() error = %v, want nil", err)
	}
}

// TestNullConn_LocalAddr_WithNilConn verifies LocalAddr returns default IP when Conn is nil
func TestNullConn_LocalAddr_WithNilConn(t *testing.T) {
	nc := &NullConn{Conn: nil}

	addr := nc.LocalAddr()

	if addr == nil {
		t.Fatal("LocalAddr() returned nil, want valid address")
	}

	ipAddr, ok := addr.(*net.IPAddr)
	if !ok {
		t.Fatalf("LocalAddr() returned type %T, want *net.IPAddr", addr)
	}

	expectedIP := net.ParseIP("127.0.0.1")
	if !ipAddr.IP.Equal(expectedIP) {
		t.Errorf("LocalAddr() IP = %v, want %v", ipAddr.IP, expectedIP)
	}
}

// TestNullConn_LocalAddr_WithWrappedConn verifies LocalAddr delegates to wrapped connection
func TestNullConn_LocalAddr_WithWrappedConn(t *testing.T) {
	// Create a mock connection with a known local address
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	// Connect to get a real connection with a local address
	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Failed to create connection: %v", err)
	}
	defer conn.Close()

	expectedAddr := conn.LocalAddr()
	nc := &NullConn{Conn: conn}

	addr := nc.LocalAddr()

	if addr == nil {
		t.Fatal("LocalAddr() returned nil, want valid address")
	}

	if addr.String() != expectedAddr.String() {
		t.Errorf("LocalAddr() = %v, want %v", addr, expectedAddr)
	}
}

// TestNullConn_RemoteAddr_WithNilConn verifies RemoteAddr returns default IP when Conn is nil
func TestNullConn_RemoteAddr_WithNilConn(t *testing.T) {
	nc := &NullConn{Conn: nil}

	addr := nc.RemoteAddr()

	if addr == nil {
		t.Fatal("RemoteAddr() returned nil, want valid address")
	}

	ipAddr, ok := addr.(*net.IPAddr)
	if !ok {
		t.Fatalf("RemoteAddr() returned type %T, want *net.IPAddr", addr)
	}

	expectedIP := net.ParseIP("127.0.0.1")
	if !ipAddr.IP.Equal(expectedIP) {
		t.Errorf("RemoteAddr() IP = %v, want %v", ipAddr.IP, expectedIP)
	}
}

// TestNullConn_RemoteAddr_WithWrappedConn verifies RemoteAddr delegates to wrapped connection
func TestNullConn_RemoteAddr_WithWrappedConn(t *testing.T) {
	// Create a mock connection with a known remote address
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	// Connect to get a real connection with a remote address
	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Failed to create connection: %v", err)
	}
	defer conn.Close()

	expectedAddr := conn.RemoteAddr()
	nc := &NullConn{Conn: conn}

	addr := nc.RemoteAddr()

	if addr == nil {
		t.Fatal("RemoteAddr() returned nil, want valid address")
	}

	if addr.String() != expectedAddr.String() {
		t.Errorf("RemoteAddr() = %v, want %v", addr, expectedAddr)
	}
}

// TestNullConn_SetDeadline verifies SetDeadline returns no error
func TestNullConn_SetDeadline_ReturnsNoError(t *testing.T) {
	nc := &NullConn{}

	tests := []struct {
		name     string
		deadline time.Time
	}{
		{
			name:     "PastDeadline",
			deadline: time.Now().Add(-1 * time.Hour),
		},
		{
			name:     "FutureDeadline",
			deadline: time.Now().Add(1 * time.Hour),
		},
		{
			name:     "ZeroDeadline",
			deadline: time.Time{},
		},
		{
			name:     "NowDeadline",
			deadline: time.Now(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := nc.SetDeadline(tt.deadline)

			if err != nil {
				t.Errorf("SetDeadline() error = %v, want nil", err)
			}
		})
	}
}

// TestNullConn_SetReadDeadline verifies SetReadDeadline returns no error
func TestNullConn_SetReadDeadline_ReturnsNoError(t *testing.T) {
	nc := &NullConn{}

	tests := []struct {
		name     string
		deadline time.Time
	}{
		{
			name:     "PastDeadline",
			deadline: time.Now().Add(-1 * time.Hour),
		},
		{
			name:     "FutureDeadline",
			deadline: time.Now().Add(1 * time.Hour),
		},
		{
			name:     "ZeroDeadline",
			deadline: time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := nc.SetReadDeadline(tt.deadline)

			if err != nil {
				t.Errorf("SetReadDeadline() error = %v, want nil", err)
			}
		})
	}
}

// TestNullConn_SetWriteDeadline verifies SetWriteDeadline returns no error
func TestNullConn_SetWriteDeadline_ReturnsNoError(t *testing.T) {
	nc := &NullConn{}

	tests := []struct {
		name     string
		deadline time.Time
	}{
		{
			name:     "PastDeadline",
			deadline: time.Now().Add(-1 * time.Hour),
		},
		{
			name:     "FutureDeadline",
			deadline: time.Now().Add(1 * time.Hour),
		},
		{
			name:     "ZeroDeadline",
			deadline: time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := nc.SetWriteDeadline(tt.deadline)

			if err != nil {
				t.Errorf("SetWriteDeadline() error = %v, want nil", err)
			}
		})
	}
}

// TestNullConn_InterfaceCompliance verifies NullConn implements net.Conn interface
func TestNullConn_InterfaceCompliance(t *testing.T) {
	var _ net.Conn = &NullConn{}
}

// TestNullConn_ConcurrentOperations verifies thread safety of NullConn operations
func TestNullConn_ConcurrentOperations(t *testing.T) {
	nc := &NullConn{}
	done := make(chan bool, 4)

	// Concurrent reads
	go func() {
		buffer := make([]byte, 10)
		for i := 0; i < 100; i++ {
			nc.Read(buffer)
		}
		done <- true
	}()

	// Concurrent writes
	go func() {
		data := []byte("test")
		for i := 0; i < 100; i++ {
			nc.Write(data)
		}
		done <- true
	}()

	// Concurrent deadline sets
	go func() {
		deadline := time.Now().Add(1 * time.Hour)
		for i := 0; i < 100; i++ {
			nc.SetDeadline(deadline)
			nc.SetReadDeadline(deadline)
			nc.SetWriteDeadline(deadline)
		}
		done <- true
	}()

	// Concurrent address checks
	go func() {
		for i := 0; i < 100; i++ {
			nc.LocalAddr()
			nc.RemoteAddr()
		}
		done <- true
	}()

	// Wait for all goroutines with timeout
	timeout := time.After(5 * time.Second)
	for i := 0; i < 4; i++ {
		select {
		case <-done:
			// Success
		case <-timeout:
			t.Fatal("Concurrent operations test timed out")
		}
	}
}

// TestNullConn_OperationSequence verifies typical usage patterns
func TestNullConn_OperationSequence(t *testing.T) {
	nc := &NullConn{}

	// Set deadlines
	deadline := time.Now().Add(1 * time.Hour)
	if err := nc.SetDeadline(deadline); err != nil {
		t.Errorf("SetDeadline() error = %v", err)
	}

	// Check addresses
	if addr := nc.LocalAddr(); addr == nil {
		t.Error("LocalAddr() returned nil")
	}
	if addr := nc.RemoteAddr(); addr == nil {
		t.Error("RemoteAddr() returned nil")
	}

	// Perform I/O operations
	buffer := make([]byte, 100)
	if _, err := nc.Read(buffer); err != nil {
		t.Errorf("Read() error = %v", err)
	}

	data := []byte("test data")
	if _, err := nc.Write(data); err != nil {
		t.Errorf("Write() error = %v", err)
	}

	// Close connection
	if err := nc.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}
