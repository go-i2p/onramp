//go:build !gen
// +build !gen

package onramp

import (
	"context"
	"strings"
	"testing"
)

func TestParseI2PAddrAndVirtualPortWithoutPort(t *testing.T) {
	addr, toPort, err := parseI2PAddrAndVirtualPort("example.b32.i2p")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if addr != "example.b32.i2p" {
		t.Fatalf("unexpected parsed addr: got %q", addr)
	}
	if toPort != 0 {
		t.Fatalf("expected toPort=0, got %d", toPort)
	}
}

func TestParseI2PAddrAndVirtualPortWithPort(t *testing.T) {
	addr, toPort, err := parseI2PAddrAndVirtualPort("example.b32.i2p:443")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if addr != "example.b32.i2p" {
		t.Fatalf("unexpected parsed addr: got %q", addr)
	}
	if toPort != 443 {
		t.Fatalf("expected toPort=443, got %d", toPort)
	}
}

func TestParseI2PAddrAndVirtualPortRejectsBadPort(t *testing.T) {
	_, _, err := parseI2PAddrAndVirtualPort("example.b32.i2p:not-a-port")
	if err == nil {
		t.Fatal("expected parse error for invalid port")
	}
	if !strings.Contains(err.Error(), "invalid destination virtual port") {
		t.Fatalf("expected invalid destination virtual port error, got: %v", err)
	}
}

func TestDialContextToPortRejectsInvalidFromPort(t *testing.T) {
	g := &Garlic{}
	_, err := g.DialContextToPort(context.Background(), "tcp", "example.b32.i2p:80", -1)
	if err == nil {
		t.Fatal("expected error for invalid FROM_PORT")
	}
	if !strings.Contains(err.Error(), "invalid FROM_PORT") {
		t.Fatalf("expected invalid FROM_PORT error, got: %v", err)
	}
}

func TestDialContextToPortRejectsInvalidDestinationPortInAddress(t *testing.T) {
	g := &Garlic{}
	_, err := g.DialContextToPort(context.Background(), "tcp", "example.b32.i2p:70000")
	if err == nil {
		t.Fatal("expected error for invalid destination port")
	}
	if !strings.Contains(err.Error(), "invalid destination virtual port") {
		t.Fatalf("expected invalid destination virtual port error, got: %v", err)
	}
}

func TestDialContextToPortRejectsNonI2PAddress(t *testing.T) {
	g := &Garlic{}
	_, err := g.DialContextToPort(context.Background(), "tcp", "example.com:443")
	if err == nil {
		t.Fatal("expected error for non-I2P address")
	}
	if !strings.Contains(err.Error(), "not an I2P address") {
		t.Fatalf("expected non-I2P address error, got: %v", err)
	}
}

func TestDialContextToPortRejectsMultipleFromPorts(t *testing.T) {
	g := &Garlic{}
	_, err := g.DialContextToPort(context.Background(), "tcp", "example.b32.i2p:443", 1000, 1001)
	if err == nil {
		t.Fatal("expected error for multiple FROM_PORT values")
	}
	if !strings.Contains(err.Error(), "expected at most one FROM_PORT") {
		t.Fatalf("expected too many FROM_PORTs error, got: %v", err)
	}
}

func TestGetOrCreatePersistentFromPortIsStable(t *testing.T) {
	g := &Garlic{}
	p1, err := g.getOrCreatePersistentFromPort()
	if err != nil {
		t.Fatalf("unexpected error selecting first persistent port: %v", err)
	}
	p2, err := g.getOrCreatePersistentFromPort()
	if err != nil {
		t.Fatalf("unexpected error selecting second persistent port: %v", err)
	}
	if p1 != p2 {
		t.Fatalf("expected stable persistent from port, got %d then %d", p1, p2)
	}
	if p1 < 1024 || p1 > 65535 {
		t.Fatalf("expected persistent from port in ephemeral range, got %d", p1)
	}
}

func TestDialToPortRejectsInvalidFromPort(t *testing.T) {
	g := &Garlic{}
	_, err := g.DialToPort("tcp", "example.b32.i2p:443", 99999)
	if err == nil {
		t.Fatal("expected error for invalid FROM_PORT")
	}
	if !strings.Contains(err.Error(), "invalid FROM_PORT") {
		t.Fatalf("expected invalid FROM_PORT error, got: %v", err)
	}
}

func TestDialContextRejectsInvalidDestinationPortInAddress(t *testing.T) {
	g := &Garlic{}
	_, err := g.DialContext(context.Background(), "tcp", "example.b32.i2p:70000")
	if err == nil {
		t.Fatal("expected error for invalid destination virtual port")
	}
	if !strings.Contains(err.Error(), "invalid destination virtual port") {
		t.Fatalf("expected invalid destination virtual port error, got: %v", err)
	}
}

func TestDialContextRejectsNonI2PAddressWithPort(t *testing.T) {
	g := &Garlic{}
	_, err := g.DialContext(context.Background(), "tcp", "example.com:443")
	if err == nil {
		t.Fatal("expected error for non-I2P address")
	}
	if !strings.Contains(err.Error(), "not an I2P address") {
		t.Fatalf("expected non-I2P address error, got: %v", err)
	}
}
