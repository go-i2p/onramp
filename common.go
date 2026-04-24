package onramp

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-i2p/logger"
)

var log = logger.GetGoI2PLogger()

//go:generate go run -tags gen ./gen.go

// GetJoinedWD returns the working directory joined with the given path.
func GetJoinedWD(dir string) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		// log.WithError(err).Error("Failed to get working directory")
		return "", err
	}
	jwd := filepath.Join(wd, dir)
	ajwd, err := filepath.Abs(jwd)
	if err != nil {
		// log.WithError(err).WithField("path", jwd).Error("Failed to get absolute path")
		return "", err
	}
	if _, err := os.Stat(ajwd); err != nil {
		// log.WithField("path", ajwd).Debug("Directory does not exist, creating")
		if err := os.MkdirAll(ajwd, 0o755); err != nil {
			// log.WithError(err).WithField("path", ajwd).Error("Failed to create directory")
			return "", err
		}
	}
	// log.WithField("path", ajwd).Debug("Successfully got joined working directory")
	return ajwd, nil
}

// keystoreMu protects the keystore path variables from concurrent access.
var keystoreMu sync.Mutex

// I2P_KEYSTORE_PATH is the place where I2P Keys will be saved.
// It defaults to the directory "i2pkeys" in the current working directory.
// Reference it by calling I2PKeystorePath() to check for errors.
// Assign before concurrent use; the accessor functions are goroutine-safe.
var I2P_KEYSTORE_PATH string

// ONION_KEYSTORE_PATH is the place where Onion Keys will be saved.
// It defaults to the directory "onionkeys" in the current working directory.
// Reference it by calling TorKeystorePath() to check for errors.
// Assign before concurrent use; the accessor functions are goroutine-safe.
var ONION_KEYSTORE_PATH string

// TLS_KEYSTORE_PATH is the place where TLS Keys will be saved.
// It defaults to the directory "tlskeys" in the current working directory.
// Reference it by calling TLSKeystorePath() to check for errors.
// Assign before concurrent use; the accessor functions are goroutine-safe.
var TLS_KEYSTORE_PATH string

// ensureKeystorePath initializes and verifies a keystore directory path.
func ensureKeystorePath(currentPath *string, defaultDir, label string) (string, error) {
	if *currentPath == "" {
		log.WithField("label", label).Debug("Keystore path is empty, attempting to reinitialize")
		path, err := GetJoinedWD(defaultDir)
		if err != nil {
			log.WithError(err).WithField("label", label).Error("Failed to reinitialize keystore path")
			return "", fmt.Errorf("%s keystore path is empty and reinitialization failed: %w", label, err)
		}
		*currentPath = path
	}
	log.WithField("path", *currentPath).Debug("Checking keystore path")
	if err := os.MkdirAll(*currentPath, 0o755); err != nil {
		log.WithError(err).WithField("path", *currentPath).Error("Failed to create keystore directory")
		return "", err
	}
	log.WithField("path", *currentPath).Debug("Keystore path verified")
	return *currentPath, nil
}

// deleteKeystorePath removes a keystore directory tree.
func deleteKeystorePath(path, label string) error {
	log.WithField("path", path).Debug("Attempting to delete keystore")
	if err := os.RemoveAll(path); err != nil {
		log.WithError(err).WithField("path", path).Error("Failed to delete keystore")
		return err
	}
	log.WithField("path", path).Debug("Successfully deleted keystore")
	return nil
}

// I2PKeystorePath returns the path to the I2P Keystore. If the
// path is not set, it returns the default path. If the path does
// not exist, it creates it.
func I2PKeystorePath() (string, error) {
	keystoreMu.Lock()
	defer keystoreMu.Unlock()
	return ensureKeystorePath(&I2P_KEYSTORE_PATH, "i2pkeys", "I2P")
}

// DeleteI2PKeyStore deletes the I2P Keystore.
func DeleteI2PKeyStore() error {
	keystoreMu.Lock()
	defer keystoreMu.Unlock()
	return deleteKeystorePath(I2P_KEYSTORE_PATH, "I2P")
	// return os.RemoveAll(I2P_KEYSTORE_PATH)
}

// TorKeystorePath returns the path to the Onion Keystore. If the
// path is not set, it returns the default path. If the path does
// not exist, it creates it.
func TorKeystorePath() (string, error) {
	keystoreMu.Lock()
	defer keystoreMu.Unlock()
	return ensureKeystorePath(&ONION_KEYSTORE_PATH, "onionkeys", "Tor")
}

// DeleteTorKeyStore deletes the Onion Keystore.
func DeleteTorKeyStore() error {
	keystoreMu.Lock()
	defer keystoreMu.Unlock()
	return deleteKeystorePath(ONION_KEYSTORE_PATH, "Tor")
	// return os.RemoveAll(ONION_KEYSTORE_PATH)
}

// TLSKeystorePath returns the path to the TLS Keystore. If the
// path is not set, it returns the default path. If the path does
// not exist, it creates it.
func TLSKeystorePath() (string, error) {
	keystoreMu.Lock()
	defer keystoreMu.Unlock()
	return ensureKeystorePath(&TLS_KEYSTORE_PATH, "tlskeys", "TLS")
}

// DeleteTLSKeyStore deletes the TLS Keystore.
func DeleteTLSKeyStore() error {
	keystoreMu.Lock()
	defer keystoreMu.Unlock()
	return deleteKeystorePath(TLS_KEYSTORE_PATH, "TLS")
	// return os.RemoveAll(TLS_KEYSTORE_PATH)
}

// parseURLHostname parses a URL-like string and returns its hostname.
func parseURLHostname(value, valueLabel string) (string, error) {
	parsedURL, err := url.Parse(value)
	if err != nil {
		log.WithError(err).WithField(valueLabel, value).Error("Failed to parse URL")
		return "", err
	}
	return parsedURL.Hostname(), nil
}

// isI2PHostname reports whether a hostname is an I2P destination.
func isI2PHostname(hostname string) bool {
	return strings.HasSuffix(hostname, ".i2p")
}

// routeByHostname dispatches a connection/listener request based on I2P hostname suffix.
func routeByHostname[T any](network, target, hostname, i2pLog, torLog string, i2pFn, torFn func(string, string) (T, error)) (T, error) {
	if isI2PHostname(hostname) {
		log.WithField("hostname", hostname).Debug(i2pLog)
		return i2pFn(network, target)
	}
	log.WithField("hostname", hostname).Debug(torLog)
	return torFn(network, target)
}

// setupNamedListener logs and creates a listener using the provided constructor.
func setupNamedListener(args []string, name string, setupLog, failLog, successLog string, constructor func(...string) (net.Listener, error)) (net.Listener, error) {
	log.WithFields(logger.Fields{
		"args": args,
		"name": name,
	}).Debug(setupLog)

	listener, err := constructor(args...)
	if err != nil {
		log.WithError(err).Error(failLog)
		return nil, err
	}

	log.Debug(successLog)
	return listener, nil
}

// createAndRegisterListener creates an instance, registers it, and opens a listener.
func createAndRegisterListener[T any](
	create func() (T, error),
	register func(T),
	listen func(T) (net.Listener, error),
	createFailLog string,
	createFailErr string,
	listenFailLog string,
	registeredLog string,
	successLog string,
) (net.Listener, error) {
	instance, err := create()
	if err != nil {
		log.WithError(err).Error(createFailLog)
		return nil, fmt.Errorf(createFailErr, err)
	}

	register(instance)
	log.Debug(registeredLog)

	listener, err := listen(instance)
	if err != nil {
		log.WithError(err).Error(listenFailLog)
		return nil, err
	}

	log.Debug(successLog)
	return listener, nil
}

// deleteKeyFile removes a key file under the provided keystore and extension.
func deleteKeyFile(tunName, keystoreLabel, fileExt string, keystoreFn func() (string, error), wrapPrefix string) error {
	log.WithField("tunnel_name", tunName).Debug("Attempting to delete " + keystoreLabel + " keys")

	keystore, err := keystoreFn()
	if err != nil {
		log.WithError(err).Error("Failed to get keystore path")
		return fmt.Errorf("%s: discovery error %v", wrapPrefix, err)
	}

	keyspath := filepath.Join(keystore, tunName+fileExt)
	if err := os.Remove(keyspath); err != nil {
		log.WithError(err).WithField("path", keyspath).Error("Failed to delete key file")
		return fmt.Errorf("%s: %v", wrapPrefix, err)
	}

	log.Debug("Successfully deleted " + keystoreLabel + " keys")
	return nil
}

// Dial returns a connection for the given network and address.
// network is ignored. If the address ends in i2p, it returns an I2P connection.
// if the address ends in anything else, it returns a Tor connection.
func Dial(network, addr string) (net.Conn, error) {
	log.WithFields(logger.Fields{
		"network": network,
		"address": addr,
	}).Debug("Attempting to dial")

	hostname, err := parseURLHostname(addr, "address")
	if err != nil {
		return nil, err
	}
	return routeByHostname(network, addr, hostname,
		"Using I2P connection for .i2p address",
		"Using Tor connection for non-i2p address",
		DialGarlic, DialOnion)
}

// Listen returns a listener for the given network and address.
// if network is i2p or garlic, it returns an I2P listener.
// if network is tor or onion, it returns an Onion listener.
// if keys ends with ".i2p", it returns an I2P listener.
func Listen(network, keys string) (net.Listener, error) {
	log.WithFields(logger.Fields{
		"network": network,
		"keys":    keys,
	}).Debug("Attempting to create listener")

	if network == "i2p" || network == "garlic" {
		log.Debug("Creating I2P listener based on network type")
		return ListenGarlic(network, keys)
	}
	if network == "tor" || network == "onion" {
		log.Debug("Creating Tor listener based on network type")
		return ListenOnion(network, keys)
	}

	hostname, err := parseURLHostname(keys, "keys")
	if err != nil {
		return nil, err
	}
	return routeByHostname(network, keys, hostname,
		"Creating I2P listener based on .i2p hostname",
		"Creating Tor listener for non-i2p hostname",
		ListenGarlic, ListenOnion)
}

// CloseAll closes all Garlic and Onion instances managed by the onramp package.
// It does not affect objects instantiated directly by an application.
// This is a convenience function that calls CloseAllGarlic() and CloseAllOnion().
func CloseAll() {
	log.Debug("Closing all managed Garlic and Onion instances")
	CloseAllGarlic()
	CloseAllOnion()
	log.Debug("All managed instances closed")
}
