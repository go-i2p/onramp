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
	listener, err := garlic.ListenTLS()
	if err != nil {
		t.Error(err)
	}
	log.WithField("listener_address", listener.Addr().String()).Debug("Garlic listener created")
	defer listener.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello, %q", r.URL.Path)
	})
	go http.Serve(listener, mux)
	garlic2, err := NewGarlic("test321", "localhost:7656", OPT_WIDE)
	if err != nil {
		t.Error(err)
	}
	defer garlic2.Close()
	Sleep(60)
	transport := http.Transport{
		Dial: garlic2.Dial,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client := &http.Client{
		Transport: &transport,
	}
	resp, err := client.Get("https://" + listener.Addr().String() + "/")
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
