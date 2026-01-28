// Package hybrid2 provides efficient datagram routing by combining Datagram2
// (authenticated, repliable, with replay protection) and Datagram3 (repliable,
// low-overhead, hash-based) protocols.
//
// The hybrid approach uses datagram2 periodically (1 in 100 messages) to
// establish and refresh sender identity, then datagram3 for the remaining
// messages to reduce overhead while maintaining sender identification.
//
// # Protocol Design
//
// The hybrid2 protocol works by leveraging the strengths of both datagram types:
//   - Datagram2: Provides authenticated source addresses but has higher overhead
//   - Datagram3: Has lower overhead but uses hash-based source identification
//
// By sending authenticated datagram2 messages periodically, receivers can build
// a mapping from datagram3 source hashes to full I2P destination addresses.
// This allows efficient bulk communication while maintaining sender identification.
//
// # Usage Pattern
//
// Sender side:
//  1. First message (and every 100th message) uses datagram2 for authentication
//  2. Remaining 99 messages use datagram3 for lower overhead
//
// Receiver side:
//  1. Datagram2 messages update the hash-to-destination mapping
//  2. Datagram3 messages resolve source via the cached mapping
//
// # Security Considerations
//
// While datagram3 sources are technically unauthenticated, the hybrid approach
// provides sender identification through periodic datagram2 authentication.
// The security model relies on:
//   - Periodic identity refresh via datagram2 (every 100 messages)
//   - Hash expiry (10 minutes) to prevent stale mappings
//   - Application-level verification when stronger guarantees are needed
//
// # Example
//
//	session, err := hybrid2.NewHybridSession(primary, "my-hybrid", nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer session.Close()
//
//	// Send data - automatically chooses datagram2 or datagram3
//	err = session.SendDatagram(data, destAddr)
//
//	// Receive data - source is resolved from hash mapping
//	dg, err := session.ReceiveDatagram()
//	fmt.Printf("Received from: %s\n", dg.Source.Base32())
package hybrid2
