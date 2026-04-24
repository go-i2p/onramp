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
	"sync"

	"github.com/go-i2p/logger"

	"github.com/cretz/bine/tor"
	"github.com/cretz/bine/torutil/ed25519"
)

var (
	torp     *tor.Tor
	torpMu   sync.Mutex // Protects torp and torpRefs
	torpRefs int        // Reference count for shared Tor instance
)

// Onion represents a structure which manages an onion service and
// a Tor client. The onion service will automatically have persistent
// keys.
type Onion struct {
	*tor.StartConf
	*tor.ListenConf
	*tor.DialConf
	context.Context
	name     string
	hasRefMu sync.Mutex // Protects hasRef flag
	hasRef   bool       // Tracks if this Onion instance has registered with torpRefs
}

func (o *Onion) getStartConf() *tor.StartConf {
	if o.StartConf == nil {
		o.StartConf = &tor.StartConf{}
	}
	return o.StartConf
}

func (o *Onion) getContext() context.Context {
	if o.Context == nil {
		o.Context = context.Background()
	}
	return o.Context
}

func (o *Onion) getListenConf() (*tor.ListenConf, error) {
	keys, err := o.Keys()
	if err != nil {
		log.WithError(err).Error("Failed to get onion service keys")
		return nil, fmt.Errorf("onramp getListenConf: %w", err)
	}
	if o.ListenConf == nil {
		o.ListenConf = &tor.ListenConf{
			Key: keys,
		}
	}
	return o.ListenConf, nil
}

func (o *Onion) getDialConf() *tor.DialConf {
	if o.DialConf == nil {
		o.DialConf = &tor.DialConf{}
	}
	return o.DialConf
}

func (o *Onion) getTor() (*tor.Tor, error) {
	// Check if this Onion instance already has a reference
	o.hasRefMu.Lock()
	alreadyHasRef := o.hasRef
	o.hasRefMu.Unlock()

	torpMu.Lock()
	defer torpMu.Unlock()

	if torp == nil {
		log.Debug("Initializing new Tor instance")
		var err error
		torp, err = tor.Start(o.getContext(), o.getStartConf())
		if err != nil {
			log.WithError(err).Error("Failed to start Tor")
			return nil, fmt.Errorf("onramp getTor: failed to start Tor: %w", err)
		}
		log.Debug("Tor instance started successfully")
	}

	// Only increment reference count once per Onion instance
	if !alreadyHasRef {
		o.hasRefMu.Lock()
		o.hasRef = true
		o.hasRefMu.Unlock()
		torpRefs++
		log.WithField("refs", torpRefs).Debug("Incremented Tor reference count")
	}
	return torp, nil
}

func (o *Onion) getDialer() (*tor.Dialer, error) {
	log.Debug("Creating new Tor dialer")
	t, err := o.getTor()
	if err != nil {
		return nil, err
	}
	dialer, err := t.Dialer(o.getContext(), o.getDialConf())
	if err != nil {
		log.WithError(err).Error("Failed to create Tor dialer")
		return nil, fmt.Errorf("onramp getDialer: failed to create dialer: %w", err)
	}
	log.Debug("Tor dialer created successfully")
	return dialer, nil
}

func (o *Onion) getName() string {
	if o.name == "" {
		o.name = "onramp-onion"
	}
	return o.name
}

// listenBase creates the underlying Tor listener used by Onion listen methods.
func (o *Onion) listenBase() (net.Listener, error) {
	t, err := o.getTor()
	if err != nil {
		return nil, err
	}
	listenConf, err := o.getListenConf()
	if err != nil {
		return nil, err
	}
	listener, err := t.Listen(o.getContext(), listenConf)
	if err != nil {
		log.WithError(err).Error("Failed to create Tor listener")
		return nil, err
	}
	return listener, nil
}

// NewListener returns a net.Listener which will listen on an onion
// address, and will automatically generate a keypair and store it.
// the args are always ignored
func (o *Onion) NewListener(n, addr string) (net.Listener, error) {
	return o.Listen(n)
}

// Listen returns a net.Listener which will listen on an onion
// address, and will automatically generate a keypair and store it.
// the args are always ignored
func (o *Onion) Listen(args ...string) (net.Listener, error) {
	return setupNamedListener(
		args,
		o.getName(),
		"Setting up Onion listener",
		"Failed to create Onion listener",
		"Successfully created Onion listener",
		o.OldListen,
	)
	// return o.OldListen(args...)
}

// OldListen returns a net.Listener which will listen on an onion
// address, and will automatically generate a keypair and store it.
// the args are always ignored
func (o *Onion) OldListen(args ...string) (net.Listener, error) {
	log.WithField("name", o.getName()).Debug("Creating Tor listener")
	listener, err := o.listenBase()
	if err != nil {
		return nil, err
	}

	log.Debug("Successfully created Tor listener")
	return listener, nil
}

// ListenTLS returns a net.Listener which will apply TLS encryption
// to the onion listener, which will not be decrypted until it reaches
// the browser
func (o *Onion) ListenTLS(args ...string) (net.Listener, error) {
	log.WithField("args", args).Debug("Setting up TLS Onion listener")
	cert, err := o.TLSKeys()
	if err != nil {
		log.WithError(err).Error("Failed to get TLS keys")
		return nil, fmt.Errorf("onramp ListenTLS: %v", err)
	}
	log.Debug("Creating base Tor listener")
	l, err := o.listenBase()
	if err != nil {
		log.WithError(err).Error("Failed to create base Tor listener")
		return nil, err
	}
	log.Debug("Wrapping Tor listener with TLS")
	return tls.NewListener(
		l,
		&tls.Config{
			Certificates: []tls.Certificate{cert},
		},
	), nil
}

// Dial returns a net.Conn to the given onion address or clearnet address.
func (o *Onion) Dial(net, addr string) (net.Conn, error) {
	log.WithFields(logger.Fields{
		"network": net,
		"address": addr,
	}).Debug("Attempting to dial via Tor")
	dialer, err := o.getDialer()
	if err != nil {
		return nil, err
	}
	conn, err := dialer.DialContext(o.getContext(), net, addr)
	if err != nil {
		log.WithError(err).Error("Failed to establish Tor connection")
		return nil, err
	}

	log.Debug("Successfully established Tor connection")
	return conn, nil
}

// Close closes the Onion Service and all associated resources.
// The shared Tor instance is only closed when all Onion instances have called Close().
func (o *Onion) Close() error {
	log.WithField("name", o.getName()).Debug("Closing Onion service")

	// Check if this instance has a reference to decrement
	o.hasRefMu.Lock()
	hasRef := o.hasRef
	if hasRef {
		o.hasRef = false // Mark as no longer having a reference
	}
	o.hasRefMu.Unlock()

	if !hasRef {
		log.Debug("Onion instance has no Tor reference, nothing to close")
		return nil
	}

	torpMu.Lock()
	defer torpMu.Unlock()

	if torp == nil {
		log.Debug("Tor not running, nothing to close")
		return nil
	}

	torpRefs--
	log.WithField("refs", torpRefs).Debug("Decremented Tor reference count")

	// Only close Tor when no more references exist
	if torpRefs <= 0 {
		err := torp.Close()
		if err != nil {
			log.WithError(err).Error("Failed to close Tor instance")
			return err
		}
		torp = nil
		torpRefs = 0 // Reset to prevent negative counts
		log.Debug("Successfully closed Tor instance")
	} else {
		log.Debug("Tor instance still in use by other Onion services")
	}

	log.Debug("Successfully closed Onion service")
	return nil
}

// Keys returns the keys for the Onion
func (o *Onion) Keys() (ed25519.KeyPair, error) {
	log.WithField("name", o.getName()).Debug("Retrieving Onion keys")

	keys, err := TorKeys(o.getName())
	if err != nil {
		log.WithError(err).Error("Failed to get Tor keys")
		return nil, err
	}

	log.Debug("Successfully retrieved Onion keys")
	return keys, nil
	// return TorKeys(o.getName())
}

// DeleteKeys deletes the keys at the given key name in the key store.
// This is permanent and irreversible, and will change the onion service
// address.
func (g *Onion) DeleteKeys() error {
	log.WithField("Onion keys", g.getName()).Debug("Deleting Onion keys")
	return DeleteOnionKeys(g.getName())
}

// NewOnion returns a new Onion object.
func NewOnion(name string) (*Onion, error) {
	return &Onion{
		name: name,
	}, nil
}

// TorKeys returns a key pair which will be stored at the given key
// name in the key store. If the key already exists, it will be
// returned. If it does not exist, it will be generated.
func TorKeys(keyName string) (keys ed25519.KeyPair, retErr error) {
	log.WithField("key_name", keyName).Debug("Getting Tor keys")
	keystore, err := TorKeystorePath()
	if err != nil {
		log.WithError(err).Error("Failed to get keystore path")
		return nil, fmt.Errorf("onramp OnionKeys: discovery error %v", err)
	}
	keysPath := filepath.Join(keystore, keyName+".tor.private")
	log.WithField("path", keysPath).Debug("Checking for existing keys")
	if _, err := os.Stat(keysPath); os.IsNotExist(err) {
		log.Debug("Generating new Tor keys")
		tkeys, err := ed25519.GenerateKey(nil)
		if err != nil {
			log.WithError(err).Error("Failed to generate onion service key")
			return nil, fmt.Errorf("onramp TorKeys: failed to generate key: %w", err)
		}
		keys = tkeys

		log.WithField("path", keysPath).Debug("Creating key file")
		f, err := os.Create(keysPath)
		if err != nil {
			log.WithError(err).Error("Failed to create Tor keys file")
			return nil, fmt.Errorf("onramp TorKeys: failed to create key file: %w", err)
		}
		defer func() {
			if err := f.Close(); retErr == nil {
				retErr = err
			}
		}()
		_, err = f.Write(tkeys.PrivateKey())
		if err != nil {
			log.WithError(err).Error("Failed to write Tor keys to disk")
			return nil, fmt.Errorf("onramp TorKeys: failed to write key file: %w", err)
		}
		log.Debug("Successfully generated and stored new keys")
	} else if err == nil {
		log.Debug("Loading existing Tor keys")
		tkeys, err := os.ReadFile(keysPath)
		if err != nil {
			log.WithError(err).Error("Failed to read Tor keys from disk")
			return nil, fmt.Errorf("onramp TorKeys: failed to read key file: %w", err)
		}
		k := ed25519.FromCryptoPrivateKey(tkeys)
		keys = k
		log.Debug("Successfully loaded existing keys")
	} else {
		log.WithError(err).Error("Failed to set up Tor keys")
		return nil, fmt.Errorf("onramp TorKeys: failed to stat key file: %w", err)
	}
	return keys, nil
}

// onions stores managed Onion instances for package-level functions.
// Initialized to prevent nil map panic when using ListenOnion/DialOnion.
var (
	onions   = make(map[string]*Onion)
	onionsMu sync.RWMutex // Protects concurrent access to onions map
)

// CloseAllOnion closes all onions managed by the onramp package. It does not
// affect objects instantiated by an app.
func CloseAllOnion() {
	onionsMu.Lock()
	defer onionsMu.Unlock()
	log.WithField("count", len(onions)).Debug("Closing all Onion services")
	for i, g := range onions {
		log.WithFields(logger.Fields{
			"index": i,
			"name":  g.name,
		}).Debug("Closing Onion service")
		// Close directly instead of calling CloseOnion to avoid recursive lock
		if err := g.Close(); err != nil {
			log.WithError(err).Error("Failed to close Onion service")
		}
		delete(onions, i)
	}

	log.Debug("All Onion services closed")
}

// CloseOnion closes the Onion at the given index. It does not affect Onion
// objects instantiated by an app.
func CloseOnion(tunName string) {
	onionsMu.Lock()
	defer onionsMu.Unlock()
	log.WithField("tunnel_name", tunName).Debug("Attempting to close Onion service")

	g, ok := onions[tunName]
	if ok {
		log.WithField("name", g.name).Debug("Found Onion service, closing")
		err := g.Close()
		if err != nil {
			log.WithError(err).Error("Failed to close Onion service")
		} else {
			log.Debug("Successfully closed Onion service")
		}
		delete(onions, tunName) // Remove from map after closing
	} else {
		log.Debug("No Onion service found for tunnel name")
	}
}

// ListenOnion returns a net.Listener for a onion structure's keys
// corresponding to a structure managed by the onramp library
// and not instantiated by an app.
func ListenOnion(network, keys string) (net.Listener, error) {
	log.WithFields(logger.Fields{
		"network": network,
		"keys":    keys,
	}).Debug("Creating new Onion listener")

	return createAndRegisterListener(
		func() (*Onion, error) {
			return NewOnion(keys)
		},
		func(instance *Onion) {
			onionsMu.Lock()
			onions[keys] = instance
			onionsMu.Unlock()
		},
		func(instance *Onion) (net.Listener, error) {
			return instance.Listen()
		},
		"Failed to create new Onion",
		"onramp Listen: %v",
		"Failed to create Onion listener",
		"Onion service registered, creating listener",
		"Successfully created Onion listener",
	)
}

// DialOnion returns a net.Conn for a onion structure's keys
// corresponding to a structure managed by the onramp library
// and not instantiated by an app.
func DialOnion(network, addr string) (net.Conn, error) {
	onionsMu.Lock()
	g, ok := onions[addr]
	onionsMu.Unlock()
	if !ok {
		var err error
		g, err = NewOnion(addr)
		if err != nil {
			return nil, fmt.Errorf("onramp Dial: %v", err)
		}
		onionsMu.Lock()
		onions[addr] = g
		onionsMu.Unlock()
	}
	return g.Dial(network, addr)
}

// DeleteOnionKeys deletes the key file at the given path as determined by
// keystore + tunName.
func DeleteOnionKeys(tunName string) error {
	// Use .tor.private extension to match TorKeys() which creates the key files
	return deleteKeyFile(tunName, "Onion", ".tor.private", TorKeystorePath, "onramp DeleteOnionKeys")
}
