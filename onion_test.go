//go:build !gen
// +build !gen

package onramp

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestBareOnion(t *testing.T) {
	log.WithField("test", "TestBareOnion").Debug("Starting test countdown")
	Sleep(5)
	onion, err := NewOnion("test123")
	if err != nil {
		t.Error(err)
	}
	defer onion.Close()
	listener, err := onion.ListenTLS()
	if err != nil {
		t.Error(err)
	}
	log.WithField("listener_address", listener.Addr().String()).Debug("Onion listener created")
	
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello, %q", r.URL.Path)
	})
	
	// Create HTTP server with proper shutdown support
	server := &http.Server{Handler: mux}
	
	// Use WaitGroup to track when server goroutine completes
	var wg sync.WaitGroup
	wg.Add(1)
	
	// Start HTTP server in goroutine
	go func() {
		defer wg.Done()
		server.Serve(listener)
	}()
	
	// Ensure we shutdown server and wait for goroutine before test exits
	defer func() {
		// Gracefully shutdown the server with timeout
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		server.Shutdown(shutdownCtx)
		// Wait for server goroutine to complete
		wg.Wait()
	}()
	
	Sleep(60)
	transport := http.Transport{
		Dial: onion.Dial,
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