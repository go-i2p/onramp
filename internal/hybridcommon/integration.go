package hybridcommon

import (
	"errors"
	"fmt"
	"io"

	sam3 "github.com/go-i2p/go-sam-go"
	"github.com/go-i2p/go-sam-go/primary"
	"github.com/go-i2p/i2pkeys"
)

// ApplyOptions applies a slice of option functions to a target value.
func ApplyOptions[T any, O ~func(*T)](target *T, opts []O) {
	for _, opt := range opts {
		opt(target)
	}
}

// SetupManagedPrimary creates a SAM connection, keys, and PRIMARY session.
func SetupManagedPrimary(samAddr, name, label string, keys **i2pkeys.I2PKeys, options []string) (*sam3.SAM, *primary.PrimarySession, error) {
	sam, err := sam3.NewSAM(samAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: creating SAM connection: %w", label, err)
	}
	if *keys == nil {
		generated, err := sam.NewKeys()
		if err != nil {
			sam.Close()
			return nil, nil, fmt.Errorf("%s: generating keys: %w", label, err)
		}
		*keys = &generated
	}
	primarySession, err := sam.NewPrimarySession(name, **keys, options)
	if err != nil {
		sam.Close()
		return nil, nil, fmt.Errorf("%s: creating primary session: %w", label, err)
	}
	return sam, primarySession, nil
}

// CloseManagedResources closes a session and any owned PRIMARY and SAM resources.
func CloseManagedResources(label, sessionLabel string, session io.Closer, primarySession *primary.PrimarySession, sam io.Closer) error {
	var closeErr error
	if session != nil {
		closeErr = errors.Join(closeErr, wrapCloseErr(session.Close(), sessionLabel))
	}
	if sam != nil {
		if primarySession != nil {
			closeErr = errors.Join(closeErr, wrapCloseErr(primarySession.Close(), "primary session"))
		}
		closeErr = errors.Join(closeErr, wrapCloseErr(sam.Close(), "SAM connection"))
	}
	if closeErr != nil {
		return fmt.Errorf("%s close: %w", label, closeErr)
	}
	return nil
}

// wrapCloseErr annotates a resource close error with the resource label.
func wrapCloseErr(err error, resource string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", resource, err)
}
