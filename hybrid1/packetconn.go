package hybrid1

import (
	"crypto/sha256"
	"encoding/binary"
	"net"
	"time"

	"github.com/go-i2p/i2pkeys"
)

// computeSenderHash computes a 32-byte SHA-256 hash from an I2P destination address.
func computeSenderHash(addr i2pkeys.I2PAddr) SenderHash {
	var hash SenderHash
	h := sha256.New()
	if bytes, err := addr.ToBytes(); err == nil {
		h.Write(bytes)
		copy(hash[:], h.Sum(nil))
	}
	return hash
}

// ReadFrom implements net.PacketConn.ReadFrom.
// It receives a datagram with i2pd framing and returns the data and source address.
func (c *Hybrid1PacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	// Check if closed.
	if c.isClosed() {
		return 0, nil, ErrSessionClosed
	}

	// Check for deadline.
	c.deadlineMu.RLock()
	deadline := c.readDeadline
	c.deadlineMu.RUnlock()

	var dg *Hybrid1Datagram

	if deadline.IsZero() {
		// No deadline, block indefinitely.
		dg, err = c.receiveBlocking()
	} else {
		// Calculate timeout duration.
		timeout := time.Until(deadline)
		if timeout <= 0 {
			return 0, nil, ErrTimeout
		}
		dg, err = c.receiveWithTimeout(timeout)
	}

	if err != nil {
		return 0, nil, err
	}

	n = copy(p, dg.Data)
	addr = &Hybrid1Addr{
		I2PAddr: dg.Source,
		Port:    dg.SourcePort,
	}
	return n, addr, nil
}

// receiveBlocking waits for a datagram without timeout.
func (c *Hybrid1PacketConn) receiveBlocking() (*Hybrid1Datagram, error) {
	select {
	case dg := <-c.session.recvChan:
		return dg, nil
	case err := <-c.session.recvErrChan:
		return nil, err
	}
}

// receiveWithTimeout waits for a datagram with a timeout.
func (c *Hybrid1PacketConn) receiveWithTimeout(timeout time.Duration) (*Hybrid1Datagram, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case dg := <-c.session.recvChan:
		return dg, nil
	case err := <-c.session.recvErrChan:
		return nil, err
	case <-timer.C:
		return nil, ErrTimeout
	}
}

// WriteTo implements net.PacketConn.WriteTo.
// It sends a datagram with i2pd framing to the specified address.
func (c *Hybrid1PacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	// Check if closed.
	if c.isClosed() {
		return 0, ErrSessionClosed
	}

	// Validate payload size.
	if len(p) > I2PD_MAX_PAYLOAD_SIZE {
		return 0, ErrPayloadTooLarge
	}

	// Extract I2P address and port from net.Addr.
	var dest i2pkeys.I2PAddr
	var destPort uint16

	switch a := addr.(type) {
	case *Hybrid1Addr:
		dest = a.I2PAddr
		destPort = a.Port
	case i2pkeys.I2PAddr:
		dest = a
		destPort = I2PD_DEFAULT_PORT
	default:
		// Try to parse as string.
		dest = i2pkeys.I2PAddr(addr.String())
		destPort = I2PD_DEFAULT_PORT
	}

	// Build i2pd-framed datagram.
	framedData := c.buildI2PDFrame(p, c.session.localPort, destPort)

	// Check for write deadline.
	c.deadlineMu.RLock()
	deadline := c.writeDeadline
	c.deadlineMu.RUnlock()

	if !deadline.IsZero() {
		timeout := time.Until(deadline)
		if timeout <= 0 {
			return 0, ErrTimeout
		}
		// Note: SAM3 doesn't have native timeout support for writes,
		// but we check the deadline before attempting the send.
	}

	// Update sender state and send.
	state := c.session.getSenderState(dest)
	state.mu.Lock()
	state.Counter++
	state.LastSendTime = time.Now()
	state.mu.Unlock()

	// Send via datagram subsession.
	_, err = c.session.datagramSub.WriteTo(framedData, dest)
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

// buildI2PDFrame creates an i2pd-framed datagram with header.
func (c *Hybrid1PacketConn) buildI2PDFrame(payload []byte, srcPort, dstPort uint16) []byte {
	frame := make([]byte, I2PD_HEADER_SIZE+len(payload))

	// Write source port (big-endian).
	binary.BigEndian.PutUint16(frame[0:2], srcPort)

	// Write destination port (big-endian).
	binary.BigEndian.PutUint16(frame[2:4], dstPort)

	// Copy payload.
	copy(frame[I2PD_HEADER_SIZE:], payload)

	return frame
}

// Close implements net.PacketConn.Close.
// This closes only this PacketConn wrapper, not the underlying session.
func (c *Hybrid1PacketConn) Close() error {
	c.closedMu.Lock()
	defer c.closedMu.Unlock()
	c.closed = true
	return nil
}

// LocalAddr implements net.PacketConn.LocalAddr.
func (c *Hybrid1PacketConn) LocalAddr() net.Addr {
	return &Hybrid1Addr{
		I2PAddr: c.session.localAddr,
		Port:    c.session.localPort,
	}
}

// SetDeadline implements net.PacketConn.SetDeadline.
func (c *Hybrid1PacketConn) SetDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	c.readDeadline = t
	c.writeDeadline = t
	return nil
}

// SetReadDeadline implements net.PacketConn.SetReadDeadline.
func (c *Hybrid1PacketConn) SetReadDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	c.readDeadline = t
	return nil
}

// SetWriteDeadline implements net.PacketConn.SetWriteDeadline.
func (c *Hybrid1PacketConn) SetWriteDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	defer c.deadlineMu.Unlock()
	c.writeDeadline = t
	return nil
}

// isClosed checks if this PacketConn wrapper has been closed.
func (c *Hybrid1PacketConn) isClosed() bool {
	c.closedMu.RLock()
	defer c.closedMu.RUnlock()
	return c.closed || c.session.IsClosed()
}
