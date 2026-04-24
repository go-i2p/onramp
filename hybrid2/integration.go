package hybrid2

import (
	"fmt"
	"net"

	sam3 "github.com/go-i2p/go-sam-go"
	"github.com/go-i2p/go-sam-go/primary"
	"github.com/go-i2p/i2pkeys"
	"github.com/go-i2p/onramp/internal/hybridcommon"
)

// GarlicIntegration provides methods to integrate hybrid2 with the Garlic struct.
// This helper enables easy hybrid2 session creation from existing infrastructure.
type GarlicIntegration struct {
	// SAM connection for session management
	sam *sam3.SAM

	// Primary session for tunnel sharing
	primary *primary.PrimarySession

	// Hybrid session instance
	hybrid *HybridSession

	// Session configuration
	name    string
	keys    *i2pkeys.I2PKeys
	options []string
}

// GarlicOption configures a GarlicIntegration instance.
type GarlicOption func(*GarlicIntegration)

// WithGarlicKeys sets the I2P keys for the integration.
func WithGarlicKeys(keys *i2pkeys.I2PKeys) GarlicOption {
	return func(g *GarlicIntegration) {
		g.keys = keys
	}
}

// WithGarlicOptions sets the SAM options for the integration.
func WithGarlicOptions(options []string) GarlicOption {
	return func(g *GarlicIntegration) {
		g.options = options
	}
}

// NewGarlicIntegration creates a new integration helper for using hybrid2 with Garlic-like patterns.
//
// Example usage:
//
//	integration, err := NewGarlicIntegration("127.0.0.1:7656", "my-hybrid",
//	    WithGarlicKeys(keys),
//	    WithGarlicOptions([]string{"inbound.length=1"}),
//	)
//	if err != nil {
//	    // handle error
//	}
//	defer integration.Close()
//
//	conn := integration.PacketConn()
//	// Use conn for network operations
func NewGarlicIntegration(samAddr, name string, opts ...GarlicOption) (*GarlicIntegration, error) {
	gi := &GarlicIntegration{
		name: name,
	}

	hybridcommon.ApplyOptions(gi, opts)

	var err error
	gi.sam, gi.primary, err = hybridcommon.SetupManagedPrimary(samAddr, name, "hybrid2 integration", &gi.keys, gi.options)
	if err != nil {
		return nil, err
	}

	// Create hybrid session
	hybridOpts := []SessionOption{}
	if gi.options != nil {
		hybridOpts = append(hybridOpts, WithOptions(gi.options))
	}
	gi.hybrid, err = NewHybridSession(gi.primary, name+"-hybrid", hybridOpts...)
	if err != nil {
		_ = hybridcommon.CloseManagedResources("hybrid2 integration", "hybrid session", nil, gi.primary, gi.sam)
		return nil, fmt.Errorf("hybrid2 integration: creating hybrid session: %w", err)
	}

	return gi, nil
}

// NewGarlicIntegrationFromPrimary creates a hybrid2 integration from an existing primary session.
// This is useful when you already have a Garlic instance with a primary session.
//
// Example usage:
//
//	// Assuming garlic.primary is your existing primary session
//	integration, err := NewGarlicIntegrationFromPrimary(garlic.Primary(), "hybrid")
//	if err != nil {
//	    // handle error
//	}
//	defer integration.Close()
func NewGarlicIntegrationFromPrimary(primarySession *primary.PrimarySession, name string, opts ...SessionOption) (*GarlicIntegration, error) {
	gi := &GarlicIntegration{
		name:    name,
		primary: primarySession,
	}

	// Create hybrid session from existing primary
	var err error
	gi.hybrid, err = NewHybridSession(primarySession, name, opts...)
	if err != nil {
		return nil, fmt.Errorf("hybrid2 integration: creating hybrid session: %w", err)
	}

	return gi, nil
}

// Close closes all resources associated with this integration.
func (gi *GarlicIntegration) Close() error {
	return hybridcommon.CloseManagedResources("hybrid2 integration", "hybrid session", gi.hybrid, gi.primary, gi.sam)
}

// Session returns the underlying hybrid session.
func (gi *GarlicIntegration) Session() *HybridSession {
	return gi.hybrid
}

// PacketConn returns a net.PacketConn for the hybrid session.
// This provides a standard Go networking interface for hybrid datagram communication.
func (gi *GarlicIntegration) PacketConn() net.PacketConn {
	return gi.hybrid.PacketConn()
}

// Addr returns the local I2P address.
func (gi *GarlicIntegration) Addr() i2pkeys.I2PAddr {
	return gi.hybrid.Addr()
}

// SendDatagram sends a datagram using the hybrid protocol.
func (gi *GarlicIntegration) SendDatagram(data []byte, dest i2pkeys.I2PAddr) error {
	return gi.hybrid.SendDatagram(data, dest)
}

// ReceiveDatagram receives a datagram from the hybrid session.
func (gi *GarlicIntegration) ReceiveDatagram() (*HybridDatagram, error) {
	return gi.hybrid.ReceiveDatagram()
}

// Keys returns the I2P keys used by this integration.
func (gi *GarlicIntegration) Keys() *i2pkeys.I2PKeys {
	return gi.keys
}

// PrimarySession returns the underlying primary session.
// This can be used to create additional subsessions if needed.
func (gi *GarlicIntegration) PrimarySession() *primary.PrimarySession {
	return gi.primary
}
