//go:build !gen
// +build !gen

package onramp

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-i2p/logger"
)

func TestBareGarlic(t *testing.T) {
	log.WithField("test", "TestBareGarlic").Debug("Starting test countdown")
	Sleep(5)
	garlic, err := NewGarlic("test123", "localhost:7656", OPT_WIDE)
	if err != nil {
		t.Error(err)
	}
	defer garlic.Close()
	listener, err := garlic.ListenTLS() // Revert back to TLS
	if err != nil {
		t.Error(err)
	}
	log.WithField("listener_address", listener.Addr().String()).Debug("Garlic listener created")
	defer listener.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.WithField("request_path", r.URL.Path).Debug("HTTP server received request")
		fmt.Fprintf(w, "Hello, %q", r.URL.Path)
		log.Debug("HTTP server sent response")
	})

	// Use custom server with logging instead of http.Serve
	server := &http.Server{
		Handler: mux,
	}

	go func() {
		log.Debug("Starting HTTP server")
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Error("HTTP server error")
		}
	}()

	// Give the HTTP server time to start accepting connections
	Sleep(5)

	garlic2, err := NewGarlic("test321", "localhost:7656", OPT_WIDE)
	if err != nil {
		t.Error(err)
	}
	defer garlic2.Close()

	// Ensure both server and client PRIMARY sessions are established
	// This replaces the crude 60-second sleep with proper session verification
	log.Debug("Setting up client PRIMARY session by creating a test listener")
	testListener, err := garlic2.ListenStream()
	if err != nil {
		t.Errorf("Failed to setup client PRIMARY session: %v", err)
		return
	}
	testListener.Close() // Close immediately as we only needed it for session setup
	log.Debug("Client PRIMARY session established")

	// Give additional time for I2P tunnels to fully stabilize
	Sleep(30)

	// Test basic I2P connectivity first, before HTTP
	serverAddr := garlic.String()
	log.WithField("server_address", serverAddr).Debug("Testing basic I2P connection")
	testConn, dialErr := garlic2.Dial("tcp", serverAddr)
	if dialErr != nil {
		t.Errorf("Failed to establish basic I2P connection: %v", dialErr)
		return
	}
	testConn.Close()
	log.Debug("Basic I2P connection successful")

	// Test if we can send a simple HTTP request manually
	rawConn, dialErr2 := garlic2.Dial("tcp", serverAddr)
	if dialErr2 != nil {
		t.Errorf("Failed to establish connection for raw HTTP test: %v", dialErr2)
		return
	}
	defer rawConn.Close()

	// Send a simple HTTP GET request
	_, writeErr := rawConn.Write([]byte("GET / HTTP/1.1\r\nHost: " + serverAddr + "\r\n\r\n"))
	if writeErr != nil {
		t.Errorf("Failed to write HTTP request: %v", writeErr)
		return
	}

	// Try to read response
	buffer := make([]byte, 1024)
	rawConn.SetReadDeadline(time.Now().Add(10 * time.Second))
	n, readErr := rawConn.Read(buffer)
	if readErr != nil {
		t.Errorf("Failed to read HTTP response: %v", readErr)
		return
	}
	log.WithField("response", string(buffer[:n])).Debug("Raw HTTP response received")

	// Now try HTTP connection with Go's HTTP client
	transport := http.Transport{
		Dial: garlic2.Dial,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client := &http.Client{
		Transport: &transport,
	}
	// Use HTTPS with TLS again
	log.WithField("server_address", serverAddr).Debug("Connecting to I2P service address")
	resp, err := client.Get("https://" + serverAddr + "/")
	if err != nil {
		t.Error(err)
		return
	}
	defer resp.Body.Close()
	log.WithField("status", resp.Status).Debug("HTTP response received")
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Error(err)
	}
	log.WithField("body", string(body)).Debug("Response body received")
	Sleep(5)
}

func Serve(listener net.Listener) {
	if err := http.Serve(listener, nil); err != nil {
		// Don't treat listener closure as fatal - this is expected during test cleanup
		if err.Error() != "use of closed network connection" &&
			!strings.Contains(err.Error(), "closed") &&
			!strings.Contains(err.Error(), "shutdown") {
			log.Fatal(err)
		}
	}
}

func Sleep(count int) {
	for i := 0; i < count; i++ {
		time.Sleep(time.Second)
		x := count - i
		log.WithFields(logger.Fields{
			"remaining_seconds": x,
			"operation":         "sleep",
		}).Debug("Waiting")
	}
}
