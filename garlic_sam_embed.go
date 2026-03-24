package onramp

import (
	"fmt"
	"net"
	"time"

	"github.com/go-i2p/go-sam-bridge/lib/embedding"
	"github.com/go-i2p/go-sam-bridge/lib/session"
)

func (g *Garlic) newEmbeddedSAMBridge() (*embedding.Bridge, error) {
	// If port 7656 is available (nothing listening), create an embedded SAM bridge
	// If something is already listening (external SAM bridge), return nil
	if checkPortAvailable(g.getAddr()) {
		bridge, err := embedding.New(
			embedding.WithListenAddr(g.getAddr()),
			// Disable the embedding-level UDP datagram listener. The
			// bridge.Server.ListenAndServe() also attempts to bind the
			// same UDP port, causing a "double-bind" failure that
			// prevents the TCP listener from ever starting.
			// go-sam-go clients send datagrams over the SAM TCP
			// protocol, so the legacy UDP port is unnecessary.
			embedding.WithDatagramPort(0),
			// Use a subsession-aware registry so that STREAM ACCEPT on
			// a new TCP connection can find subsessions that were added
			// to a PRIMARY session via SESSION ADD. The default registry
			// only stores top-level sessions; subsessions are kept inside
			// the PrimarySession and invisible to other connections.
			embedding.WithRegistry(&subsessionAwareRegistry{
				Registry: session.NewRegistry(),
			}),
		)
		if err != nil {
			return nil, err
		}
		return bridge, nil
	}
	return nil, nil
}

func checkPortAvailable(addr string) bool {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// waitForSAMReady polls the SAM address until it accepts a TCP connection
// or the timeout elapses.  This is needed because Bridge.Start() is
// non-blocking — the TCP accept loop runs in a background goroutine that
// may not be ready immediately.
func waitForSAMReady(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("SAM bridge at %s not ready after %s", addr, timeout)
}

// subsessionAwareRegistry wraps a session.Registry and additionally searches
// through PRIMARY sessions' subsession lists when Get() doesn't find a
// direct match. This works around a gap in go-sam-bridge where subsessions
// created via SESSION ADD are stored inside the PrimarySession but not
// registered in the global registry, making them invisible to STREAM
// ACCEPT/CONNECT commands on separate TCP connections.
type subsessionAwareRegistry struct {
	session.Registry
}

func (r *subsessionAwareRegistry) Get(id string) session.Session {
	// Direct lookup first (covers top-level sessions).
	if s := r.Registry.Get(id); s != nil {
		return s
	}
	// Fall through: search subsessions of every registered PRIMARY session.
	for _, sessID := range r.Registry.All() {
		s := r.Registry.Get(sessID)
		if primary, ok := s.(session.PrimarySession); ok {
			if sub := primary.Subsession(id); sub != nil {
				return sub
			}
		}
	}
	return nil
}
