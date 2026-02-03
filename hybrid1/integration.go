package hybrid1

import (
	"fmt"
	"net"

	sam3 "github.com/go-i2p/go-sam-go"
	"github.com/go-i2p/go-sam-go/primary"
	"github.com/go-i2p/i2pkeys"
)

// GarlicIntegration provides methods to integrate hybrid1 with the Garlic struct.
// This helper enables easy hybrid1 session creation from existing infrastructure.
type GarlicIntegration struct {
	// SAM connection for session management.
	sam *sam3.SAM

	// Primary session for tunnel sharing.
	primary *primary.PrimarySession

	// Hybrid1 session instance.
	hybrid1 *Hybrid1Session

	// Session configuration.
	name    string
	keys    *i2pkeys.I2PKeys
	options []string
}

// IntegrationOption configures a GarlicIntegration instance.
type IntegrationOption func(*GarlicIntegration)

// WithIntegrationKeys sets the I2P keys for the integration.
func WithIntegrationKeys(keys *i2pkeys.I2PKeys) IntegrationOption {
	return func(g *GarlicIntegration) {
		g.keys = keys
	}
}

// WithIntegrationOptions sets the SAM options for the integration.
func WithIntegrationOptions(options []string) IntegrationOption {
	return func(g *GarlicIntegration) {
		g.options = options
	}
}

// NewGarlicIntegration creates a new integration helper for using hybrid1 with Garlic-like patterns.
//
// Example usage:
//
//	integration, err := NewGarlicIntegration("127.0.0.1:7656", "my-hybrid1",
//	    WithIntegrationKeys(keys),
//	    WithIntegrationOptions([]string{"inbound.length=1"}),
//	)
//	if err != nil {
//	    // handle error
//	}
//	defer integration.Close()
//
//	conn := integration.PacketConn()
//	// Use conn for network operations
func NewGarlicIntegration(samAddr, name string, opts ...IntegrationOption) (*GarlicIntegration, error) {
	gi := &GarlicIntegration{
		name: name,
	}

	// Apply options.
	for _, opt := range opts {
		opt(gi)
	}

	// Create SAM connection.
	var err error
	gi.sam, err = sam3.NewSAM(samAddr)
	if err != nil {
		return nil, fmt.Errorf("hybrid1 integration: creating SAM connection: %w", err)
	}

	// Generate keys if not provided.
	if gi.keys == nil {
		keys, err := gi.sam.NewKeys()
		if err != nil {
			gi.sam.Close()
			return nil, fmt.Errorf("hybrid1 integration: generating keys: %w", err)
		}
		gi.keys = &keys
	}

	// Create primary session.
	gi.primary, err = gi.sam.NewPrimarySession(name, *gi.keys, gi.options)
	if err != nil {
		gi.sam.Close()
		return nil, fmt.Errorf("hybrid1 integration: creating primary session: %w", err)
	}

	// Create hybrid1 session.
	hybrid1Opts := []SessionOption{}
	if gi.options != nil {
		hybrid1Opts = append(hybrid1Opts, WithOptions(gi.options))
	}
	gi.hybrid1, err = NewHybrid1Session(gi.primary, name+"-hybrid1", hybrid1Opts...)
	if err != nil {
		gi.primary.Close()
		gi.sam.Close()
		return nil, fmt.Errorf("hybrid1 integration: creating hybrid1 session: %w", err)
	}

	return gi, nil
}

// NewGarlicIntegrationFromPrimary creates a hybrid1 integration from an existing primary session.
// This is useful when you already have a Garlic instance with a primary session.
//
// Example usage:
//
//	// Use existing primary session from Garlic
//	primary, err := garlic.Primary()
//	if err != nil {
//	    // handle error
//	}
//	integration, err := NewGarlicIntegrationFromPrimary(primary, "hybrid1")
//	if err != nil {
//	    // handle error
//	}
//	defer integration.Close()
func NewGarlicIntegrationFromPrimary(primarySession *primary.PrimarySession, name string, opts ...SessionOption) (*GarlicIntegration, error) {
	gi := &GarlicIntegration{
		name:    name,
		primary: primarySession,
	}

	// Create hybrid1 session from existing primary.
	var err error
	gi.hybrid1, err = NewHybrid1Session(primarySession, name, opts...)
	if err != nil {
		return nil, fmt.Errorf("hybrid1 integration: creating hybrid1 session: %w", err)
	}

	return gi, nil
}

// Close closes all resources associated with this integration.
func (gi *GarlicIntegration) Close() error {
	var errors []error

	// Close hybrid1 session.
	if gi.hybrid1 != nil {
		if err := gi.hybrid1.Close(); err != nil {
			errors = append(errors, fmt.Errorf("hybrid1 session: %w", err))
		}
	}

	// Only close primary and SAM if we created them.
	if gi.sam != nil {
		if gi.primary != nil {
			if err := gi.primary.Close(); err != nil {
				errors = append(errors, fmt.Errorf("primary session: %w", err))
			}
		}

		if err := gi.sam.Close(); err != nil {
			errors = append(errors, fmt.Errorf("SAM connection: %w", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("hybrid1 integration close: %v", errors)
	}
	return nil
}

// Session returns the underlying hybrid1 session.
func (gi *GarlicIntegration) Session() *Hybrid1Session {
	return gi.hybrid1
}

// PacketConn returns a net.PacketConn for the hybrid1 session.
// This provides a standard Go networking interface for i2pd-compatible datagram communication.
func (gi *GarlicIntegration) PacketConn() net.PacketConn {
	return gi.hybrid1.PacketConn()
}

// Addr returns the local I2P address.
func (gi *GarlicIntegration) Addr() i2pkeys.I2PAddr {
	return gi.hybrid1.Addr()
}

// SendDatagram sends a datagram using the hybrid1 protocol with i2pd framing.
func (gi *GarlicIntegration) SendDatagram(data []byte, dest i2pkeys.I2PAddr) error {
	conn := gi.hybrid1.PacketConn()
	_, err := conn.WriteTo(data, &Hybrid1Addr{I2PAddr: dest})
	return err
}

// SendDatagramWithPort sends a datagram with explicit port specification.
func (gi *GarlicIntegration) SendDatagramWithPort(data []byte, dest i2pkeys.I2PAddr, destPort uint16) error {
	conn := gi.hybrid1.PacketConn()
	_, err := conn.WriteTo(data, &Hybrid1Addr{I2PAddr: dest, Port: destPort})
	return err
}

// ReceiveDatagram receives a datagram from the hybrid1 session.
func (gi *GarlicIntegration) ReceiveDatagram() (*Hybrid1Datagram, error) {
	select {
	case dg := <-gi.hybrid1.recvChan:
		return dg, nil
	case err := <-gi.hybrid1.recvErrChan:
		return nil, err
	}
}

// Keys returns the I2P keys used by this integration.
func (gi *GarlicIntegration) Keys() *i2pkeys.I2PKeys {
	return gi.keys
}

// PrimarySession returns the underlying primary session.
func (gi *GarlicIntegration) PrimarySession() *primary.PrimarySession {
	return gi.primary
}
