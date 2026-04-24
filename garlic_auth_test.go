//go:build !gen
// +build !gen

package onramp

import (
	"testing"

	sam3 "github.com/go-i2p/go-sam-go"
)

func TestCreateSAMSessionWithoutAuthUsesDefaultFactory(t *testing.T) {
	originalNewSAMSession := newSAMSession
	originalNewSAMSessionWithAuth := newSAMSessionWithAuth
	defer func() {
		newSAMSession = originalNewSAMSession
		newSAMSessionWithAuth = originalNewSAMSessionWithAuth
	}()

	var defaultCalls int
	var authCalls int
	newSAMSession = func(address string) (*sam3.SAM, error) {
		defaultCalls++
		if address != "127.0.0.1:7656" {
			t.Fatalf("expected address 127.0.0.1:7656, got %q", address)
		}
		return &sam3.SAM{}, nil
	}
	newSAMSessionWithAuth = func(address, user, password string) (*sam3.SAM, error) {
		authCalls++
		return &sam3.SAM{}, nil
	}

	if _, err := createSAMSession("127.0.0.1:7656", "", ""); err != nil {
		t.Fatalf("createSAMSession() returned error: %v", err)
	}
	if defaultCalls != 1 {
		t.Fatalf("expected unauthenticated factory to be called once, got %d", defaultCalls)
	}
	if authCalls != 0 {
		t.Fatalf("expected authenticated factory to not be called, got %d", authCalls)
	}
}

func TestCreateSAMSessionWithAuthUsesAuthFactory(t *testing.T) {
	originalNewSAMSession := newSAMSession
	originalNewSAMSessionWithAuth := newSAMSessionWithAuth
	defer func() {
		newSAMSession = originalNewSAMSession
		newSAMSessionWithAuth = originalNewSAMSessionWithAuth
	}()

	var defaultCalls int
	var authCalls int
	newSAMSession = func(address string) (*sam3.SAM, error) {
		defaultCalls++
		return &sam3.SAM{}, nil
	}
	newSAMSessionWithAuth = func(address, user, password string) (*sam3.SAM, error) {
		authCalls++
		if address != "127.0.0.1:7656" {
			t.Fatalf("expected address 127.0.0.1:7656, got %q", address)
		}
		if user != "alice" {
			t.Fatalf("expected user alice, got %q", user)
		}
		if password != "secret" {
			t.Fatalf("expected password secret, got %q", password)
		}
		return &sam3.SAM{}, nil
	}

	if _, err := createSAMSession("127.0.0.1:7656", "alice", "secret"); err != nil {
		t.Fatalf("createSAMSession() returned error: %v", err)
	}
	if authCalls != 1 {
		t.Fatalf("expected authenticated factory to be called once, got %d", authCalls)
	}
	if defaultCalls != 0 {
		t.Fatalf("expected unauthenticated factory to not be called, got %d", defaultCalls)
	}
}

func TestGarlicSamSessionWithAuthCachesSession(t *testing.T) {
	originalNewSAMSession := newSAMSession
	originalNewSAMSessionWithAuth := newSAMSessionWithAuth
	defer func() {
		newSAMSession = originalNewSAMSession
		newSAMSessionWithAuth = originalNewSAMSessionWithAuth
	}()

	var authCalls int
	expectedSAM := &sam3.SAM{}
	newSAMSession = func(address string) (*sam3.SAM, error) {
		t.Fatal("unexpected unauthenticated SAM factory call")
		return nil, nil
	}
	newSAMSessionWithAuth = func(address, user, password string) (*sam3.SAM, error) {
		authCalls++
		return expectedSAM, nil
	}

	garlic := &Garlic{addr: "127.0.0.1:7656", samUser: "alice", samPassword: "secret"}

	first, err := garlic.samSession()
	if err != nil {
		t.Fatalf("first samSession() returned error: %v", err)
	}
	second, err := garlic.samSession()
	if err != nil {
		t.Fatalf("second samSession() returned error: %v", err)
	}
	if authCalls != 1 {
		t.Fatalf("expected authenticated factory to be called once, got %d", authCalls)
	}
	if first != expectedSAM || second != expectedSAM {
		t.Fatal("samSession() did not cache and reuse the created SAM session")
	}
}
