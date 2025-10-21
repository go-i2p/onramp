//go:build !gen
// +build !gen

package onramp

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	sam3 "github.com/go-i2p/go-sam-go"
	"github.com/go-i2p/go-sam-go/primary"
	"github.com/go-i2p/i2pkeys"
	"github.com/go-i2p/logger"
)

// Garlic is a ready-made I2P streaming manager. Once initialized it always
// has a valid I2PKeys and uses SAMv3.3 PRIMARY sessions for efficient tunnel sharing.
//
// The struct uses PRIMARY sessions with subsessions, allowing multiple connection types
// (stream and datagram) to share the same I2P tunnels, reducing resource overhead.
type Garlic struct {
	// PRIMARY session fields (SAMv3.3+)
	// These enable efficient tunnel sharing across multiple subsessions
	primary     *primary.PrimarySession     // Master session managing shared tunnels
	streamSub   *primary.StreamSubSession   // Stream subsession for TCP-like connections
	datagramSub *primary.DatagramSubSession // Datagram subsession for UDP-like messaging

	// Common fields
	ServiceKeys *i2pkeys.I2PKeys
	*sam3.SAM
	name        string
	addr        string
	opts        []string
	AddrMode    int
	TorrentMode bool
}

const (
	DEST_BASE32           = 0
	DEST_HASH             = 1
	DEST_HASH_BYTES       = 2
	DEST_BASE32_TRUNCATED = 3
	DEST_BASE64           = 4
	DEST_BASE64_BYTES     = 5
)

func (g *Garlic) Network() string {
	if g.streamSub != nil {
		return "tcp"
	} else {
		return "udp"
	}
}

func (g *Garlic) addrString(addr string) string {
	if g.TorrentMode {
		return addr + ".i2p"
	}
	return addr
}

func (g *Garlic) String() string {
	var r string
	switch g.AddrMode {
	case DEST_HASH:
		r = g.ServiceKeys.Address.DestHash().Hash()
	case DEST_HASH_BYTES:
		hash := g.ServiceKeys.Address.DestHash()
		r = string(hash[:])
	case DEST_BASE32_TRUNCATED:
		r = strings.TrimSuffix(g.ServiceKeys.Address.Base32(), ".b32.i2p")
	case DEST_BASE32:
		r = g.ServiceKeys.Address.Base32()
	case DEST_BASE64:
		r = g.ServiceKeys.Address.Base64()
	case DEST_BASE64_BYTES:
		r = string(g.ServiceKeys.Address.Bytes())
	default:
		r = g.ServiceKeys.Address.DestHash().Hash()
	}
	return g.addrString(r) // r //strings.TrimLeft(strings.TrimRight(r, "\n"), "\n") //strings.TrimSpace(r)
}

func (g *Garlic) getName() string {
	if g.name == "" {
		return "onramp-garlic"
	}
	return g.name
}

func (g *Garlic) getAddr() string {
	if g.addr == "" {
		return "localhost:7656"
	}
	return g.addr
}

func (g *Garlic) getOptions() []string {
	if g.opts == nil {
		return OPT_DEFAULTS
	}
	return g.opts
}

// getStreamSubID returns a unique ID for the stream subsession.
// The ID is based on the tunnel name to ensure uniqueness while remaining
// predictable for debugging purposes.
func (g *Garlic) getStreamSubID() string {
	return g.getName() + "-stream"
}

// getDatagramSubID returns a unique ID for the datagram subsession.
// The ID is based on the tunnel name to ensure uniqueness while remaining
// predictable for debugging purposes.
func (g *Garlic) getDatagramSubID() string {
	return g.getName() + "-datagram"
}

func (g *Garlic) samSession() (*sam3.SAM, error) {
	if g.SAM == nil {
		log.WithField("address", g.getAddr()).Debug("Creating new SAM session")
		var err error
		g.SAM, err = sam3.NewSAM(g.getAddr())
		if err != nil {
			log.WithError(err).Error("Failed to create SAM session")
			return nil, fmt.Errorf("onramp samSession: %v", err)
		}
		log.Debug("SAM session created successfully")
	}
	return g.SAM, nil
}

// setupPrimarySession creates or returns the existing PRIMARY session.
// This method uses SAMv3.3 PRIMARY session functionality to enable efficient
// tunnel sharing across multiple subsessions. The PRIMARY session is created
// with Ed25519 (signature type 7) for modern cryptography.
//
// PRIMARY session creation takes 2-5 minutes for I2P tunnel establishment.
// This method should be called early in the initialization process to front-load
// the tunnel setup overhead. Subsessions created later will attach to these
// tunnels nearly instantly.
//
// Returns the PRIMARY session or an error if creation fails.
func (g *Garlic) setupPrimarySession() (*primary.PrimarySession, error) {
	if g.primary == nil {
		log.WithField("name", g.getName()).Debug("Setting up PRIMARY session")

		// Get or generate I2P keys
		var err error
		g.ServiceKeys, err = g.Keys()
		if err != nil {
			log.WithError(err).Error("Failed to get keys for PRIMARY session")
			return nil, fmt.Errorf("onramp setupPrimarySession: %v", err)
		}

		log.WithField("address", g.ServiceKeys.Address.Base32()).Debug("Creating PRIMARY session with keys")

		// Create PRIMARY session with Ed25519 signature type (type 7)
		// This provides modern cryptography and is the recommended default
		// sam3.SAM.NewPrimarySession returns *primary.PrimarySession
		g.primary, err = g.SAM.NewPrimarySession(
			g.getName(),
			*g.ServiceKeys,
			g.getOptions(),
		)
		if err != nil {
			log.WithError(err).Error("Failed to create PRIMARY session")
			return nil, fmt.Errorf("onramp setupPrimarySession: %v", err)
		}

		log.Debug("PRIMARY session created successfully")
	}
	return g.primary, nil
}

// setupStreamSubSession creates or returns the stream subsession.
// This method creates a stream subsession attached to the PRIMARY session,
// enabling TCP-like reliable connections that share the PRIMARY session's tunnels.
//
// Stream subsessions are created with PORT=0 (any port) by default, which is
// suitable for most applications. The subsession uses the same I2P identity as
// the PRIMARY session but operates independently for connection management.
//
// Returns the stream subsession or an error if creation fails.
func (g *Garlic) setupStreamSubSession() (*primary.StreamSubSession, error) {
	if g.streamSub == nil {
		log.WithField("name", g.getName()).Debug("Setting up stream subsession")

		// Ensure PRIMARY session exists first
		if _, err := g.setupPrimarySession(); err != nil {
			return nil, err
		}

		// Create stream subsession with PORT=0 (any port)
		// This is the recommended default for single-service applications
		var err error
		subOpts := append(g.getOptions(), "PORT=0")
		g.streamSub, err = g.primary.NewStreamSubSession(g.getStreamSubID(), subOpts)
		if err != nil {
			log.WithError(err).Error("Failed to create stream subsession")
			return nil, fmt.Errorf("onramp setupStreamSubSession: %v", err)
		}

		log.Debug("Stream subsession created successfully")
	}
	return g.streamSub, nil
}

// setupDatagramSubSession creates or returns the datagram subsession.
// This method creates a datagram subsession attached to the PRIMARY session,
// enabling UDP-like messaging that shares the PRIMARY session's tunnels.
//
// Datagram subsessions use authenticated messaging (DATAGRAM protocol) which
// includes full source destination information. Per SAMv3.3 specification,
// DATAGRAM subsessions require a PORT parameter. If not provided, PORT=0
// (any port) is added automatically.
//
// Returns the datagram subsession or an error if creation fails.
func (g *Garlic) setupDatagramSubSession() (*primary.DatagramSubSession, error) {
	if g.datagramSub == nil {
		log.WithField("name", g.getName()).Debug("Setting up datagram subsession")

		// Ensure PRIMARY session exists first
		if _, err := g.setupPrimarySession(); err != nil {
			return nil, err
		}

		// Create datagram subsession with PORT=0 (any port)
		// Per SAMv3.3 spec, DATAGRAM subsessions require a PORT parameter
		var err error
		subOpts := append(g.getOptions(), "PORT=0")
		g.datagramSub, err = g.primary.NewDatagramSubSession(g.getDatagramSubID(), subOpts)
		if err != nil {
			log.WithError(err).Error("Failed to create datagram subsession")
			return nil, fmt.Errorf("onramp setupDatagramSubSession: %v", err)
		}

		log.Debug("Datagram subsession created successfully")
	}
	return g.datagramSub, nil
}

// NewListener returns a net.Listener for the Garlic structure's I2P keys.
// accepts a variable list of arguments, arguments after the first one are ignored.
func (g *Garlic) NewListener(n, addr string) (net.Listener, error) {
	log.WithFields(logger.Fields{
		"network": n,
		"address": addr,
		"name":    g.getName(),
	}).Debug("Creating new listener")
	listener, err := g.Listen(n)
	if err != nil {
		log.WithError(err).Error("Failed to create listener")
		return nil, err
	}

	log.Debug("Successfully created listener")
	return listener, nil
	// return g.Listen(n)
}

// Listen returns a net.Listener for the Garlic structure's I2P keys.
// accepts a variable list of arguments, arguments after the first one are ignored.
func (g *Garlic) Listen(args ...string) (net.Listener, error) {
	log.WithFields(logger.Fields{
		"args": args,
		"name": g.getName(),
	}).Debug("Setting up listener")

	listener, err := g.OldListen(args...)
	if err != nil {
		log.WithError(err).Error("Failed to create listener")
		return nil, err
	}

	log.Debug("Successfully created listener")
	return listener, nil
	// return g.OldListen(args...)
}

// OldListen returns a net.Listener for the Garlic structure's I2P keys.
// accepts a variable list of arguments, arguments after the first one are ignored.
func (g *Garlic) OldListen(args ...string) (net.Listener, error) {
	log.WithField("args", args).Debug("Starting OldListen")
	if len(args) > 0 {
		protocol := args[0]
		log.WithField("protocol", protocol).Debug("Checking protocol type")
		// if args[0] == "tcp" || args[0] == "tcp6" || args[0] == "st" || args[0] == "st6" {
		if protocol == "tcp" || protocol == "tcp6" || protocol == "st" || protocol == "st6" {
			log.Debug("Using TCP stream listener")
			return g.ListenStream()
			//} else if args[0] == "udp" || args[0] == "udp6" || args[0] == "dg" || args[0] == "dg6" {
		} else if protocol == "udp" || protocol == "udp6" || protocol == "dg" || protocol == "dg6" {
			log.Debug("Using UDP datagram listener")
			pk, err := g.ListenPacket()
			if err != nil {
				log.WithError(err).Error("Failed to create packet listener")
				return nil, err
			}
			log.Debug("Successfully created datagram session")
			return pk.(*sam3.DatagramSession).Listen()
		}

	}
	log.Debug("No protocol specified, defaulting to stream listener")
	return g.ListenStream()
}

// ListenStream returns a net.Listener for the Garlic structure's I2P keys.
// This method uses PRIMARY sessions with stream subsessions for efficient
// tunnel sharing. Multiple stream connections share the same I2P tunnels,
// reducing resource overhead significantly.
func (g *Garlic) ListenStream() (net.Listener, error) {
	log.Debug("Setting up stream listener")
	var err error

	// Initialize SAM connection
	if g.SAM, err = g.samSession(); err != nil {
		log.WithError(err).Error("Failed to create SAM session for stream listener")
		return nil, fmt.Errorf("onramp ListenStream: %v", err)
	}

	// Setup stream subsession (which ensures PRIMARY session exists)
	if g.streamSub, err = g.setupStreamSubSession(); err != nil {
		log.WithError(err).Error("Failed to setup stream subsession")
		return nil, fmt.Errorf("onramp ListenStream: %v", err)
	}

	// Create listener from subsession
	log.Debug("Creating new stream listener from subsession")
	listener, err := g.streamSub.StreamSession.Listen()
	if err != nil {
		log.WithError(err).Error("Failed to create stream listener")
		return nil, fmt.Errorf("onramp ListenStream: %v", err)
	}
	log.Debug("Stream listener created successfully")

	return listener, nil
}

// ListenPacket returns a net.PacketConn for the Garlic structure's I2P keys.
// This method now uses PRIMARY sessions with datagram subsessions for efficient
// tunnel sharing. The datagram subsession provides authenticated UDP-like messaging
// that shares tunnels with other subsessions.
func (g *Garlic) ListenPacket() (net.PacketConn, error) {
	log.Debug("Setting up packet connection")
	var err error

	// Initialize SAM connection
	if g.SAM, err = g.samSession(); err != nil {
		log.WithError(err).Error("Failed to create SAM session for packet connection")
		return nil, fmt.Errorf("onramp ListenPacket: %v", err)
	}

	// Setup datagram subsession (which ensures PRIMARY session exists)
	if g.datagramSub, err = g.setupDatagramSubSession(); err != nil {
		log.WithError(err).Error("Failed to setup datagram subsession")
		return nil, fmt.Errorf("onramp ListenPacket: %v", err)
	}

	log.Debug("Packet connection successfully established")
	// DatagramSession from subsession is already a PacketConn
	return g.datagramSub.DatagramSession, nil
}

// ListenTLS returns a net.Listener for the Garlic structure's I2P keys,
// which also uses TLS either for additional encryption, authentication,
// or browser-compatibility.
func (g *Garlic) ListenTLS(args ...string) (net.Listener, error) {
	log.WithField("args", args).Debug("Starting TLS listener")
	listener, err := g.Listen(args...)
	if err != nil {
		log.WithError(err).Error("Failed to create base listener")
		return nil, err
	}
	cert, err := g.TLSKeys()
	if err != nil {
		log.WithError(err).Error("Failed to get TLS keys")
		return nil, fmt.Errorf("onramp ListenTLS: %v", err)
	}
	if len(args) > 0 {
		protocol := args[0]
		log.WithField("protocol", protocol).Debug("Creating TLS listener for protocol")

		// if args[0] == "tcp" || args[0] == "tcp6" || args[0] == "st" || args[0] == "st6" {
		if protocol == "tcp" || protocol == "tcp6" || protocol == "st" || protocol == "st6" {
			log.Debug("Creating TLS stream listener")
			return tls.NewListener(
				listener,
				&tls.Config{
					Certificates: []tls.Certificate{cert},
				},
			), nil
			//} else if args[0] == "udp" || args[0] == "udp6" || args[0] == "dg" || args[0] == "dg6" {
		} else if protocol == "udp" || protocol == "udp6" || protocol == "dg" || protocol == "dg6" {
			log.Debug("Creating TLS datagram listener")
			// Get datagram subsession
			var err error
			if g.datagramSub, err = g.setupDatagramSubSession(); err != nil {
				log.WithError(err).Error("Failed to setup datagram subsession")
				return nil, fmt.Errorf("onramp ListenTLS: %v", err)
			}
			ln, err := g.datagramSub.DatagramSession.Listen()
			if err != nil {
				log.WithError(err).Error("Failed to create datagram listener")
				return nil, err
			}
			return tls.NewListener(
				ln,
				&tls.Config{
					Certificates: []tls.Certificate{cert},
				},
			), nil
		}

	}
	log.Debug("No protocol specified, using stream listener")
	return tls.NewListener(
		listener,
		&tls.Config{
			Certificates: []tls.Certificate{cert},
		},
	), nil
}

// Dial returns a net.Conn for the Garlic structure's I2P keys.
// This method now uses PRIMARY sessions with stream subsessions for efficient
// tunnel sharing. All outbound connections share the same tunnels as the listener.
func (g *Garlic) Dial(net, addr string) (net.Conn, error) {
	log.WithFields(logger.Fields{
		"network": net,
		"address": addr,
	}).Debug("Attempting to dial")

	if !strings.Contains(addr, ".i2p") {
		log.Debug("Non-I2P address detected, returning null connection")
		return &NullConn{}, nil
	}

	var err error
	if g.SAM, err = g.samSession(); err != nil {
		log.WithError(err).Error("Failed to create SAM session")
		return nil, fmt.Errorf("onramp Dial: %v", err)
	}

	// Setup stream subsession instead of old stream session
	if g.streamSub, err = g.setupStreamSubSession(); err != nil {
		log.WithError(err).Error("Failed to setup stream subsession")
		return nil, fmt.Errorf("onramp Dial: %v", err)
	}

	log.Debug("Attempting to establish connection")
	conn, err := g.streamSub.StreamSession.Dial(addr)
	if err != nil {
		log.WithError(err).Error("Failed to establish connection")
		return nil, err
	}

	log.Debug("Successfully established connection")
	return conn, nil
}

// DialContext returns a net.Conn for the Garlic structure's I2P keys with context support.
// This method now uses PRIMARY sessions with stream subsessions for efficient
// tunnel sharing. The context allows cancellation and timeout control.
func (g *Garlic) DialContext(ctx context.Context, net, addr string) (net.Conn, error) {
	log.WithFields(logger.Fields{
		"network": net,
		"address": addr,
	}).Debug("Attempting to dial with context")

	if !strings.Contains(addr, ".i2p") {
		log.Debug("Non-I2P address detected, returning null connection")
		return &NullConn{}, nil
	}

	var err error
	if g.SAM, err = g.samSession(); err != nil {
		log.WithError(err).Error("Failed to create SAM session")
		return nil, fmt.Errorf("onramp DialContext: %v", err)
	}

	// Setup stream subsession instead of old stream session
	if g.streamSub, err = g.setupStreamSubSession(); err != nil {
		log.WithError(err).Error("Failed to setup stream subsession")
		return nil, fmt.Errorf("onramp DialContext: %v", err)
	}

	log.Debug("Attempting to establish connection with context")
	conn, err := g.streamSub.StreamSession.DialContext(ctx, addr)
	if err != nil {
		log.WithError(err).Error("Failed to establish connection")
		return nil, err
	}

	log.Debug("Successfully established connection")
	return conn, nil
}

// Close closes the Garlic structure's sessions and listeners.
// When using PRIMARY sessions, closing the PRIMARY session automatically
// closes all subsessions. Legacy session cleanup is maintained for backward
// compatibility.
func (g *Garlic) Close() error {
	log.WithField("name", g.getName()).Debug("Closing Garlic sessions")

	var errors []error

	// Close PRIMARY session (automatically closes all subsessions)
	if g.primary != nil {
		if err := g.primary.Close(); err != nil {
			log.WithError(err).Error("Failed to close PRIMARY session")
			errors = append(errors, fmt.Errorf("PRIMARY session: %v", err))
		} else {
			log.Debug("PRIMARY session closed successfully")
		}
	}

	// Close SAM connection
	if g.SAM != nil {
		if err := g.SAM.Close(); err != nil {
			log.WithError(err).Error("Failed to close SAM connection")
			errors = append(errors, fmt.Errorf("SAM connection: %v", err))
		} else {
			log.Debug("SAM connection closed successfully")
		}
	}

	// Clear references
	g.primary = nil
	g.streamSub = nil
	g.datagramSub = nil

	if len(errors) > 0 {
		return fmt.Errorf("onramp Close: %v", errors)
	}

	log.Debug("All sessions closed successfully")
	return nil
}

// Keys returns the I2PKeys for the Garlic structure. If none
// exist, they are created and stored.
func (g *Garlic) Keys() (*i2pkeys.I2PKeys, error) {
	log.WithFields(logger.Fields{
		"name":    g.getName(),
		"address": g.getAddr(),
	}).Debug("Retrieving I2P keys")

	keys, err := I2PKeys(g.getName(), g.getAddr())
	if err != nil {
		log.WithError(err).Error("Failed to get I2P keys")
		return &i2pkeys.I2PKeys{}, fmt.Errorf("onramp Keys: %v", err)
	}
	log.Debug("Successfully retrieved I2P keys")
	return &keys, nil
}

func (g *Garlic) DeleteKeys() error {
	// return DeleteGarlicKeys(g.getName())
	log.WithField("name", g.getName()).Debug("Attempting to delete Garlic keys")
	err := DeleteGarlicKeys(g.getName())
	if err != nil {
		log.WithError(err).Error("Failed to delete Garlic keys")
	}
	log.Debug("Successfully deleted Garlic keys")
	return err
}

// NewGarlic returns a new Garlic struct. It is immediately ready to use with
// I2P streaming using SAMv3.3 PRIMARY sessions.
//
// Design Decision: PRIMARY sessions are created lazily on first use of
// ListenStream(), Dial(), or ListenPacket() rather than eagerly in the constructor.
// This ensures:
// - Fast initialization (2-5 minutes saved on startup)
// - PRIMARY sessions are only created when actually needed
// - Applications can set up multiple Garlic instances without waiting
//
// PRIMARY session creation takes 2-5 minutes for I2P tunnel establishment.
// Subsequent subsession creation is nearly instant once PRIMARY tunnels exist.
func NewGarlic(tunName, samAddr string, options []string) (*Garlic, error) {
	log.WithFields(logger.Fields{
		"tunnel_name": tunName,
		"sam_address": samAddr,
		"options":     options,
	}).Debug("Creating new Garlic instance")

	g := new(Garlic)
	g.name = tunName
	g.addr = samAddr
	g.opts = options
	var err error
	if g.SAM, err = g.samSession(); err != nil {
		log.WithError(err).Error("Failed to create SAM session")
		return nil, fmt.Errorf("onramp NewGarlic: %v", err)
	}
	// PRIMARY sessions are created lazily on first use (e.g., ListenStream, Dial, ListenPacket)
	// to maintain fast initialization. Session setup takes 2-5 minutes for I2P tunnel establishment.

	log.Debug("Successfully created new Garlic instance")
	return g, nil
}

// DeleteGarlicKeys deletes the key file at the given path as determined by
// keystore + tunName.
// This is permanent and irreversible, and will change the onion service
// address.
func DeleteGarlicKeys(tunName string) error {
	log.WithField("tunnel_name", tunName).Debug("Attempting to delete Garlic keys")
	keystore, err := I2PKeystorePath()
	if err != nil {
		log.WithError(err).Error("Failed to get keystore path")
		return fmt.Errorf("onramp DeleteGarlicKeys: discovery error %v", err)
	}
	keyspath := filepath.Join(keystore, tunName+".i2p.private")
	log.WithField("path", keyspath).Debug("Deleting key file")
	if err := os.Remove(keyspath); err != nil {
		log.WithError(err).WithField("path", keyspath).Error("Failed to delete key file")
		return fmt.Errorf("onramp DeleteGarlicKeys: %v", err)
	}
	log.Debug("Successfully deleted Garlic keys")
	return nil
}

// I2PKeys returns the I2PKeys at the keystore directory for the given
// tunnel name. If none exist, they are created and stored.
func I2PKeys(tunName, samAddr string) (i2pkeys.I2PKeys, error) {
	log.WithFields(logger.Fields{
		"tunnel_name": tunName,
		"sam_address": samAddr,
	}).Debug("Looking up I2P keys")

	keystore, err := I2PKeystorePath()
	if err != nil {
		log.WithError(err).Error("Failed to get keystore path")
		return i2pkeys.I2PKeys{}, fmt.Errorf("onramp I2PKeys: discovery error %v", err)
	}
	keyspath := filepath.Join(keystore, tunName+".i2p.private")
	log.WithField("path", keyspath).Debug("Checking for existing keys")
	info, err := os.Stat(keyspath)
	if info != nil {
		if info.Size() == 0 {
			log.WithField("path", keyspath).Debug("Keystore empty, will regenerate keys")
			log.Println("onramp I2PKeys: keystore empty, re-generating keys")
		} else {
			log.WithField("path", keyspath).Debug("Found existing keystore")
		}
	}
	if err != nil {
		log.WithField("path", keyspath).Debug("Keys not found, generating new keys")
		sam, err := sam3.NewSAM(samAddr)
		if err != nil {
			log.WithError(err).Error("Failed to create SAM connection")
			return i2pkeys.I2PKeys{}, fmt.Errorf("onramp I2PKeys: SAM error %v", err)
		}
		log.Debug("SAM connection established")
		keys, err := sam.NewKeys(tunName)
		if err != nil {
			log.WithError(err).Error("Failed to generate new keys")
			return i2pkeys.I2PKeys{}, fmt.Errorf("onramp I2PKeys: keygen error %v", err)
		}
		log.Debug("New keys generated successfully")
		if err = i2pkeys.StoreKeys(keys, keyspath); err != nil {
			log.WithError(err).WithField("path", keyspath).Error("Failed to store generated keys")
			return i2pkeys.I2PKeys{}, fmt.Errorf("onramp I2PKeys: store error %v", err)
		}
		log.WithField("path", keyspath).Debug("Successfully stored new keys")
		return keys, nil
	} else {
		log.WithField("path", keyspath).Debug("Loading existing keys")
		keys, err := i2pkeys.LoadKeys(keyspath)
		if err != nil {
			log.WithError(err).WithField("path", keyspath).Error("Failed to load existing keys")
			return i2pkeys.I2PKeys{}, fmt.Errorf("onramp I2PKeys: load error %v", err)
		}
		log.Debug("Successfully loaded existing keys")
		return keys, nil
	}
}

var garlics map[string]*Garlic

// CloseAllGarlic closes all garlics managed by the onramp package. It does not
// affect objects instantiated by an app.
func CloseAllGarlic() {
	log.WithField("count", len(garlics)).Debug("Closing all Garlic connections")
	for i, g := range garlics {
		log.WithFields(logger.Fields{
			"index": i,
			"name":  g.name,
		}).Debug("Closing Garlic connection")

		log.Println("Closing garlic", g.name)
		CloseGarlic(i)
	}
	log.Debug("All Garlic connections closed")
}

// CloseGarlic closes the Garlic at the given index. It does not affect Garlic
// objects instantiated by an app.
func CloseGarlic(tunName string) {
	log.WithField("tunnel_name", tunName).Debug("Attempting to close Garlic connection")
	g, ok := garlics[tunName]
	if ok {
		log.Debug("Found Garlic connection, closing")
		// g.Close()
		err := g.Close()
		if err != nil {
			log.WithError(err).Error("Error closing Garlic connection")
		} else {
			log.Debug("Successfully closed Garlic connection")
		}
	} else {
		log.Debug("No Garlic connection found for tunnel name")
	}
}

// SAM_ADDR is the default I2P SAM address. It can be overridden by the
// struct or by changing this variable.
var SAM_ADDR = "127.0.0.1:7656"

// ListenGarlic returns a net.Listener for a garlic structure's keys
// corresponding to a structure managed by the onramp library
// and not instantiated by an app.
func ListenGarlic(network, keys string) (net.Listener, error) {
	log.WithFields(logger.Fields{
		"network":  network,
		"keys":     keys,
		"sam_addr": SAM_ADDR,
	}).Debug("Creating new Garlic listener")
	g, err := NewGarlic(keys, SAM_ADDR, OPT_DEFAULTS)
	if err != nil {
		log.WithError(err).Error("Failed to create new Garlic")
		return nil, fmt.Errorf("onramp Listen: %v", err)
	}
	garlics[keys] = g
	log.Debug("Successfully created Garlic listener")
	return g.Listen()
}

// DialGarlic returns a net.Conn for a garlic structure's keys
// corresponding to a structure managed by the onramp library
// and not instantiated by an app.
func DialGarlic(network, addr string) (net.Conn, error) {
	log.WithFields(logger.Fields{
		"network":  network,
		"address":  addr,
		"sam_addr": SAM_ADDR,
	}).Debug("Creating new Garlic connection")

	g, err := NewGarlic(addr, SAM_ADDR, OPT_DEFAULTS)
	if err != nil {
		log.WithError(err).Error("Failed to create new Garlic")
		return nil, fmt.Errorf("onramp Dial: %v", err)
	}
	garlics[addr] = g
	log.WithField("address", addr).Debug("Attempting to dial")
	conn, err := g.Dial(network, addr)
	if err != nil {
		log.WithError(err).Error("Failed to dial connection")
		return nil, err
	}

	log.Debug("Successfully established Garlic connection")
	return conn, nil
	// return g.Dial(network, addr)
}
