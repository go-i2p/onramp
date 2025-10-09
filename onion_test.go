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
	fmt.Println("TestBareOnion Countdown")
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
	log.Println("listener:", listener.Addr().String())
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
	fmt.Println("Status:", resp.Status)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Error(err)
	}
	fmt.Println("Body:", string(body))
	Sleep(5)
}
