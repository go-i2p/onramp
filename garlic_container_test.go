//go:build container
// +build container

package onramp

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestContainerEmbeddedRouterBootstrap verifies that the embedded go-i2p
// router starts when no external SAM bridge is running. Inside a clean
// container ports 7654 (I2CP) and 7656 (SAM) are free, so the embedded
// router must initialise automatically.
func TestContainerEmbeddedRouterBootstrap(t *testing.T) {
	t.Log("Creating Garlic with embedded router (no external SAM expected)")

	garlic, err := NewGarlic("embed-bootstrap", SAM_ADDR, OPT_SMALL)
	if err != nil {
		t.Fatalf("NewGarlic failed: %v", err)
	}
	defer garlic.Close()

	if garlic.Bridge == nil {
		t.Fatal("Expected embedded Bridge to be non-nil inside container")
	}
	t.Log("Embedded SAM bridge created successfully")

	// The second Garlic should reuse the existing SAM bridge (port occupied).
	garlic2, err := NewGarlic("embed-bootstrap-2", SAM_ADDR, OPT_SMALL)
	if err != nil {
		t.Fatalf("Second NewGarlic failed: %v", err)
	}
	defer garlic2.Close()

	if garlic2.Bridge != nil {
		t.Fatal("Expected second Garlic's Bridge to be nil (port already taken)")
	}
	t.Log("Second Garlic correctly reuses existing SAM bridge")
}

// TestContainerStreamRoundTrip creates two Garlic instances inside the
// container, sets up a listener on one and dials from the other, then
// exchanges data over the I2P stream.
func TestContainerStreamRoundTrip(t *testing.T) {
	const payload = "hello from embedded go-i2p"

	t.Log("Setting up server Garlic")
	server, err := NewGarlic("container-srv", SAM_ADDR, OPT_SMALL)
	if err != nil {
		t.Fatalf("NewGarlic (server): %v", err)
	}
	defer server.Close()

	listener, err := server.ListenStream()
	if err != nil {
		t.Fatalf("ListenStream: %v", err)
	}
	defer listener.Close()
	serverAddr := server.String()
	t.Logf("Server listening at %s", serverAddr)

	// Accept one connection in background.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := listener.Accept()
		if err != nil {
			t.Errorf("Accept: %v", err)
			return
		}
		defer conn.Close()
		buf := make([]byte, len(payload))
		if _, err := io.ReadFull(conn, buf); err != nil {
			t.Errorf("server ReadFull: %v", err)
			return
		}
		if string(buf) != payload {
			t.Errorf("server got %q, want %q", buf, payload)
			return
		}
		if _, err := conn.Write(buf); err != nil {
			t.Errorf("server Write: %v", err)
		}
	}()

	// Give the server tunnels time to publish.
	t.Log("Waiting for I2P tunnels to stabilise…")
	waitForTunnels(t, 120*time.Second)

	t.Log("Setting up client Garlic")
	client, err := NewGarlic("container-cli", SAM_ADDR, OPT_SMALL)
	if err != nil {
		t.Fatalf("NewGarlic (client): %v", err)
	}
	defer client.Close()

	// Establish the client's own PRIMARY session so it has tunnels.
	tmpL, err := client.ListenStream()
	if err != nil {
		t.Fatalf("client ListenStream (session bootstrap): %v", err)
	}
	tmpL.Close()

	waitForTunnels(t, 60*time.Second)

	t.Log("Dialling server from client")
	conn, err := client.Dial("tcp", serverAddr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte(payload)); err != nil {
		t.Fatalf("client Write: %v", err)
	}
	echo := make([]byte, len(payload))
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	if _, err := io.ReadFull(conn, echo); err != nil {
		t.Fatalf("client ReadFull: %v", err)
	}
	if string(echo) != payload {
		t.Fatalf("client got %q, want %q", echo, payload)
	}
	t.Log("Stream round-trip succeeded")
	wg.Wait()
}

// TestContainerHTTPOverI2P runs an HTTP server behind a TLS I2P listener
// and performs a full HTTP GET from a separate Garlic client, all inside
// the container using only the embedded router.
func TestContainerHTTPOverI2P(t *testing.T) {
	t.Log("Setting up HTTP server Garlic")
	srvGarlic, err := NewGarlic("container-http-srv", SAM_ADDR, OPT_SMALL)
	if err != nil {
		t.Fatalf("NewGarlic (http server): %v", err)
	}
	defer srvGarlic.Close()

	listener, err := srvGarlic.ListenTLS()
	if err != nil {
		t.Fatalf("ListenTLS: %v", err)
	}
	defer listener.Close()
	serverAddr := srvGarlic.String()
	t.Logf("HTTP server at %s", serverAddr)

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "pong")
	})
	httpSrv := &http.Server{Handler: mux}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := httpSrv.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Errorf("HTTP Serve: %v", err)
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpSrv.Shutdown(ctx)
		wg.Wait()
	}()

	t.Log("Waiting for server tunnels…")
	waitForTunnels(t, 120*time.Second)

	t.Log("Setting up HTTP client Garlic")
	cliGarlic, err := NewGarlic("container-http-cli", SAM_ADDR, OPT_SMALL)
	if err != nil {
		t.Fatalf("NewGarlic (http client): %v", err)
	}
	defer cliGarlic.Close()

	// Bootstrap client PRIMARY session.
	tmpL, err := cliGarlic.ListenStream()
	if err != nil {
		t.Fatalf("client ListenStream (session bootstrap): %v", err)
	}
	tmpL.Close()

	waitForTunnels(t, 60*time.Second)

	transport := &http.Transport{
		Dial: cliGarlic.Dial,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	defer transport.CloseIdleConnections()
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   120 * time.Second,
	}

	t.Log("Sending HTTP GET /ping")
	resp, err := httpClient.Get("https://" + serverAddr + "/ping")
	if err != nil {
		t.Fatalf("HTTP GET: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if strings.TrimSpace(string(body)) != "pong" {
		t.Fatalf("got body %q, want %q", body, "pong")
	}
	t.Logf("HTTP round-trip succeeded: %d %s", resp.StatusCode, body)
}

// TestContainerPacketConn exercises the datagram (PacketConn) path using
// the embedded router. Two Garlic instances exchange a UDP-style message.
func TestContainerPacketConn(t *testing.T) {
	const payload = "datagram-test-payload"

	t.Log("Setting up receiver Garlic")
	recvGarlic, err := NewGarlic("container-dgram-recv", SAM_ADDR, OPT_SMALL)
	if err != nil {
		t.Fatalf("NewGarlic (recv): %v", err)
	}
	defer recvGarlic.Close()

	pc, err := recvGarlic.ListenPacket()
	if err != nil {
		t.Fatalf("ListenPacket: %v", err)
	}
	defer pc.Close()
	recvAddr := recvGarlic.String()
	t.Logf("Receiver at %s", recvAddr)

	// Read one datagram in background.
	var wg sync.WaitGroup
	wg.Add(1)
	var recvErr error
	var recvBuf []byte
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		pc.SetReadDeadline(time.Now().Add(5 * time.Minute))
		n, _, err := pc.ReadFrom(buf)
		if err != nil {
			recvErr = err
			return
		}
		recvBuf = buf[:n]
	}()

	t.Log("Waiting for receiver tunnels…")
	waitForTunnels(t, 120*time.Second)

	t.Log("Setting up sender Garlic")
	sendGarlic, err := NewGarlic("container-dgram-send", SAM_ADDR, OPT_SMALL)
	if err != nil {
		t.Fatalf("NewGarlic (send): %v", err)
	}
	defer sendGarlic.Close()

	spc, err := sendGarlic.ListenPacket()
	if err != nil {
		t.Fatalf("sender ListenPacket: %v", err)
	}
	defer spc.Close()

	waitForTunnels(t, 60*time.Second)

	// Resolve the receiver address.
	dest, err := net.ResolveUDPAddr("udp", recvAddr)
	if err != nil {
		// If ResolveUDPAddr doesn't work with .i2p names, use the raw string
		// packaged as an Addr. PacketConn implementations in go-sam-go accept
		// I2P address strings directly.
		t.Logf("ResolveUDPAddr failed (expected for I2P): %v — using raw addr", err)
	}
	var sendAddr net.Addr
	if dest != nil {
		sendAddr = dest
	} else {
		sendAddr = i2pAddr(recvAddr)
	}

	t.Log("Sending datagram")
	_, err = spc.WriteTo([]byte(payload), sendAddr)
	if err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	wg.Wait()
	if recvErr != nil {
		t.Fatalf("ReadFrom: %v", recvErr)
	}
	if string(recvBuf) != payload {
		t.Fatalf("received %q, want %q", recvBuf, payload)
	}
	t.Log("Datagram round-trip succeeded")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// waitForTunnels sleeps in a logged countdown giving I2P tunnels time to
// publish. The embedded router needs time to build tunnels even for local
// communication.
func waitForTunnels(t *testing.T, d time.Duration) {
	t.Helper()
	seconds := int(d.Seconds())
	for i := 0; i < seconds; i++ {
		time.Sleep(time.Second)
		remaining := seconds - i
		if remaining%10 == 0 {
			t.Logf("  tunnel wait: %ds remaining", remaining)
		}
	}
}

// i2pAddr is a minimal net.Addr wrapper so we can pass a raw string to WriteTo.
type i2pAddr string

func (a i2pAddr) Network() string { return "i2p" }
func (a i2pAddr) String() string  { return string(a) }
