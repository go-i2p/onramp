// Package hybrid1 provides i2pd-compatible UDP datagram protocol support for I2P.
//
// Hybrid1 is the i2pd compatibility mode for onramp's datagram functionality. It wraps
// SAM3 datagram sessions with i2pd-style protocol framing, enabling interoperability
// with i2pd-based applications.
//
// # Protocol Overview
//
// Hybrid1 operates at the net.PacketConn abstraction level, providing:
//   - i2pd-compatible datagram framing and addressing
//   - Port encoding in datagram headers per i2pd specification
//   - Transparent bridging between i2pd protocol and SAM3 datagrams
//   - Standard net.PacketConn interface for application compatibility
//
// # Usage
//
// Hybrid1 can be used via explicit methods on the Garlic struct:
//
//	garlic, err := onramp.NewGarlic("myapp", "127.0.0.1:7656", nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer garlic.Close()
//
//	// Explicit Hybrid1 mode (i2pd-compatible)
//	conn, err := garlic.ListenPacketHybrid1()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Use like any other net.PacketConn
//	buf := make([]byte, 64*1024)
//	n, addr, err := conn.ReadFrom(buf)
//
// Or via the ListenPacketWithMode method:
//
//	conn, err := garlic.ListenPacketWithMode(onramp.HYBRID_MODE_V1)
//
// # Hybrid1 vs Hybrid2
//
// Hybrid2 (the default) uses native SAM3 datagrams and is recommended for Go-to-Go
// communication. It provides better performance and simpler implementation.
//
// Hybrid1 should be used when:
//   - Communicating with i2pd-based applications
//   - Interoperability with non-Go I2P implementations is required
//   - Matching i2pd's datagram frame structure is necessary
//
// # Protocol Compatibility
//
//   - Hybrid1 to i2pd: Full compatibility (primary use case)
//   - Hybrid1 to Hybrid2: Compatible via SAM protocol layer
//   - Hybrid2 to Hybrid2: Full compatibility, optimal performance
//
// # Thread Safety
//
// Hybrid1PacketConn is safe for concurrent use from multiple goroutines.
// It follows the same concurrency guarantees as net.PacketConn.
//
// # Buffer Management
//
// The I2P UDP MTU is 64KB. Hybrid1 allocates appropriately sized buffers
// and handles i2pd frame overhead internally.
package hybrid1
