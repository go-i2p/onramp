//go:build !gen
// +build !gen

package onramp

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"testing"
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
	defer listener.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello, %q", r.URL.Path)
	})
	go http.Serve(listener, mux)
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
