onramp
======

[![Go Report Card](https://goreportcard.com/badge/github.com/go-i2p/onramp)](https://goreportcard.com/report/github.com/go-i2p/onramp)

High-level, easy-to-use listeners and clients for I2P and onion URL's from Go.
Provides only the most widely-used functions in a basic way. It expects nothing
from the users, an otherwise empty instance of the structs will listen and dial
I2P Streaming and Tor TCP sessions successfully.

In all cases, it assumes that keys are "persistent" in that they are managed
maintained between usages of the same application in the same configuration.
This means that hidden services will maintain their identities, and that clients
will always have the same return addresses. If you don't want this behavior,
make sure to delete the "keystore" when your app closes or when your application
needs to cycle keys by calling the `Garlic.DeleteKeys()` or `Onion.DeleteKeys()`
function. For more information, check out the [godoc](http://pkg.go.dev/github.com/go-i2p/onramp).

- **[Source Code](https://github.com/go-i2p/onramp)**

STATUS: This project is maintained. I will respond to issues, pull requests, and feature requests within a few days.

Usage
-----

Basic usage is designed to be very simple, import the package and instantiate
a struct and you're ready to go.

For more extensive examples, see: [EXAMPLE](EXAMPLE.md)

### I2P(Garlic) Usage:

When using it to manage an I2P session, set up an `onramp.Garlic`
struct.

```Go

package main

import (
	"log"

	"github.com/go-i2p/onramp"
)

func main() {
	garlic := &onramp.Garlic{}
	defer garlic.Close()
	listener, err := garlic.Listen()
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
}
```

#### Embedded SAM Bridge

The `Garlic` struct supports an embedded SAM bridge feature. When `NewGarlic()` 
is called and no external SAM bridge is listening on port 7656, onramp will 
automatically attempt to create an embedded SAM bridge using the 
`github.com/go-i2p/go-sam-bridge/lib/embedding` package. This allows applications 
to operate without requiring an external I2P router in some scenarios.

**Behavior:**
- If port 7656 is already in use (external I2P router running), the embedded bridge is not created
- If port 7656 is available, an embedded bridge is created and used automatically
- The embedded bridge is managed by the `Garlic.Bridge` field

#### Automatic Session Cleanup

All Garlic instances created via `NewGarlic()` automatically register for cleanup
to prevent orphaned SAM sessions when your program exits unexpectedly (e.g., crashes,
signals, or failing to call `Close()`). This is enabled by default.

**Cleanup mechanisms:**
- Signal handlers catch SIGINT, SIGTERM, and SIGHUP to clean up sessions before exit
- Runtime finalizers provide best-effort cleanup during garbage collection
- Custom cleanup hooks can be registered via `RegisterCleanupHook(func())`

If you need to disable automatic cleanup for a specific instance, use `DisableAutoCleanup(garlic)`.

### Tor(Onion) Usage:

When using it to manage a Tor session, set up an `onramp.Onion`
struct.

```Go

package main

import (
	"log"

	"github.com/go-i2p/onramp"
)

func main() {
	onion := &onramp.Onion{}
	defer onion.Close()
	listener, err := onion.Listen()
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
}
```

### Hybrid2 Datagram Protocol (Advanced):

The `hybrid2` sub-package provides an efficient datagram protocol for I2P
that balances authentication overhead with throughput. It uses a 1:99 ratio
of authenticated (datagram2) to low-overhead (datagram3) messages.

```Go
package main

import (
	"log"
	"time"

	"github.com/go-i2p/onramp"
	"github.com/go-i2p/onramp/hybrid2"
)

func main() {
	// Create a hybrid session using the garlic integration helper
	// This connects to the local SAM bridge and creates I2P tunnels
	// onramp.SAM_ADDR defaults to "127.0.0.1:7656"
	integration, err := hybrid2.NewGarlicIntegration(onramp.SAM_ADDR, "my-hybrid")
	if err != nil {
		log.Fatal(err)
	}
	defer integration.Close()

	// Get a standard net.PacketConn for UDP-like operations
	conn := integration.PacketConn()

	// Set read deadline for timeout support
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	// Use like any other PacketConn
	buf := make([]byte, 4096)
	n, addr, err := conn.ReadFrom(buf)
	if err != nil {
		log.Printf("Read error: %v", err)
		return
	}
	log.Printf("Received %d bytes from %s", n, addr)
}
```

The hybrid2 protocol automatically:
- Sends authenticated datagram2 messages every 100th send
- Sends low-overhead datagram3 messages for all other sends
- Maintains sender hash mappings for efficient message routing
- Provides deadline support via `SetReadDeadline()` and `SetWriteDeadline()`

## Verbosity ##

Logging is provided by the `github.com/go-i2p/logger` package, which offers structured logging with advanced debugging features.

### Logging Configuration

By default, logging is **disabled** for zero-impact performance. Enable logging using environment variables:

**Log Levels:**

- **Debug** - Detailed information for development and troubleshooting
```shell
export DEBUG_I2P=debug
```

- **Warn** - Important warnings about potential issues
```shell
export DEBUG_I2P=warn
```

- **Error** - Only serious errors and failures
```shell
export DEBUG_I2P=error
```

If `DEBUG_I2P` is set to an unrecognized value, it defaults to "debug" level.

### Fast-Fail Mode for Testing

Enable fast-fail mode to catch warnings and errors during testing. When enabled, any warning or error will cause the application to exit immediately:

```shell
export WARNFAIL_I2P=true
```

This is particularly useful in CI/CD pipelines and during development to ensure no issues are overlooked.

### Example: Running with Debug Logging

```shell
# Run with debug logging enabled
DEBUG_I2P=debug go run your-app.go

# Run with fast-fail mode for testing
DEBUG_I2P=debug WARNFAIL_I2P=true go test ./...
```

### Structured Logging Features

The logger provides structured logging with contextual fields, making logs searchable and analyzable:

- **WithField()** / **WithFields()** - Add contextual metadata to log entries
- **WithError()** - Include error context with stack traces
- Zero overhead when logging is disabled
- Proper log level filtering based on environment configuration

For more details, see the [logger package documentation](https://github.com/go-i2p/logger).


## Contributing

See CONTRIBUTING.md for more information.

## License

This project is licensed under the MIT license, see LICENSE for more information.
