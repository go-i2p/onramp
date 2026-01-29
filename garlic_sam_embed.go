package onramp

import (
	"net"

	"github.com/go-i2p/go-sam-bridge/lib/embedding"
)

func (g *Garlic) newEmbeddedSAMBridge() (*embedding.Bridge, error) {
	// If port 7656 is available (nothing listening), create an embedded SAM bridge
	// If something is already listening (external SAM bridge), return nil
	if checkPortAvailable(g.getAddr()) {
		bridge, err := embedding.New(
			embedding.WithListenAddr(g.getAddr()),
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
