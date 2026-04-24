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
	garlic, err := onramp.NewGarlic("my-app", onramp.SAM_ADDR, nil)
	if err != nil {
		log.Fatal(err)
	}
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

#### Virtual Port Dialing (`FROM_PORT` / `TO_PORT`)

`Garlic` supports SAM virtual ports for stream dialing. Use these when one I2P
destination multiplexes multiple logical services (for example, virtual port
80 and 443 on the same destination key).

Pass the remote virtual port in the destination string as `destination:port`.
The local `FROM_PORT` is optional.

```go
// Remote TO_PORT=443 (from addr), explicit local FROM_PORT=12345
conn, err := garlic.DialToPort("tcp", "example.b32.i2p:443", 12345)
if err != nil {
	log.Fatal(err)
}
defer conn.Close()

// Remote TO_PORT=443, local FROM_PORT selected once at random and reused
conn2, err := garlic.DialToPort("tcp", "example.b32.i2p:443")
```

For HTTP clients, adapt `DialContextToPort` into `http.Transport.DialContext`:

```go
transport := &http.Transport{
	DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		// addr from net/http is already host:port, e.g. "example.b32.i2p:443"
		return garlic.DialContextToPort(ctx, network, addr)
	},
}
client := &http.Client{Transport: transport}
```

Plain `Dial()` and `DialContext()` continue to use the default behavior (`FROM_PORT=0`, `TO_PORT=0`).

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
	onion, err := onramp.NewOnion("my-app")
	if err != nil {
		log.Fatal(err)
	}
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
that balances authentication overhead with throughput. It uses a 1:499 ratio
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
- Sends authenticated datagram2 messages every 500th send (1:499 ratio)
- Sends low-overhead datagram3 messages for all other sends
- Uses time-based refresh to prevent hash mapping expiry on idle connections
- Maintains sender hash mappings for efficient message routing
- Provides deadline support via `SetReadDeadline()` and `SetWriteDeadline()`

### Hybrid1 Datagram Protocol (i2pd-Compatible):

The `hybrid1` sub-package provides i2pd-compatible datagram protocol support.
Use this mode when interoperating with i2pd-based applications.

```Go
package main

import (
	"log"
	"time"

	"github.com/go-i2p/onramp"
)

func main() {
	// Create a Garlic instance
	garlic, err := onramp.NewGarlic("my-hybrid1-app", onramp.SAM_ADDR, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer garlic.Close()

	// Option 1: Explicit Hybrid1 method (i2pd-compatible)
	conn, err := garlic.ListenPacketHybrid1()
	if err != nil {
		log.Fatal(err)
	}

	// Option 2: Using mode constant
	// conn, err := garlic.ListenPacketWithMode(onramp.HYBRID_MODE_V1)

	// Set read deadline for timeout support
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	// Use like any other PacketConn
	buf := make([]byte, 64*1024) // i2pd max MTU
	n, addr, err := conn.ReadFrom(buf)
	if err != nil {
		log.Printf("Read error: %v", err)
		return
	}
	log.Printf("Received %d bytes from %s", n, addr)
}
```

**Hybrid Mode Selection:**

| Mode | Constant | Method | Use Case |
|------|----------|--------|----------|
| Hybrid2 (Default) | `HYBRID_MODE_V2` | `ListenPacket()` or `ListenPacketHybrid2()` | Go-to-Go communication, optimal performance |
| Hybrid1 (i2pd) | `HYBRID_MODE_V1` | `ListenPacketHybrid1()` | i2pd interoperability |

**When to use Hybrid1:**
- Communicating with i2pd-based applications
- Interoperability with non-Go I2P implementations
- Matching i2pd's datagram frame structure is required

**When to use Hybrid2 (Recommended):**
- Go-to-Go communication
- Most flexibility
- Native SAM3 datagram support

### Configuration Presets

`OPT_DEFAULTS`, `OPT_WIDE`, `OPT_HUGE`, `OPT_LARGE`, `OPT_MEDIUM`, and `OPT_SMALL` control
the number of I2P tunnels created for a `Garlic` session. More tunnels provide greater
redundancy and anonymity but consume more router resources and bandwidth.

```go
// Use a high-bandwidth preset for a busy server
garlic, err := onramp.NewGarlic("myservice", onramp.SAM_ADDR, onramp.OPT_WIDE)

// Use a minimal preset for a lightweight client
garlic, err := onramp.NewGarlic("myclient", onramp.SAM_ADDR, onramp.OPT_SMALL)
```

| Preset | Tunnels | Best For |
|--------|---------|----------|
| `OPT_SMALL` | Minimal | Lightweight clients, low-traffic services |
| `OPT_DEFAULTS` | Balanced | Most applications (default) |
| `OPT_MEDIUM` | Moderate | Services with moderate traffic |
| `OPT_WIDE` | Many | High-traffic servers requiring path diversity |
| `OPT_LARGE` | Many+ | Busy services needing strong redundancy |
| `OPT_HUGE` | Maximum | Maximum-throughput, resource-rich servers |

### Bidirectional Proxy

`OnrampProxy` exposes a local service as an I2P or Tor hidden service by bridging
an inbound `net.Listener` to a remote address. The `Proxy()` package-level convenience
function uses a default instance.

```go
// Expose a local HTTP server on I2P
garlic, err := onramp.NewGarlic("myproxy", onramp.SAM_ADDR, onramp.OPT_DEFAULTS)
if err != nil {
    log.Fatal(err)
}
listener, err := garlic.Listen()
if err != nil {
    log.Fatal(err)
}
// Bridge the I2P listener to a local service running on :8080
err = onramp.Proxy(listener, "localhost:8080")
```

### NullConn

`NullConn` is a no-op `net.Conn` that discards writes and returns `io.EOF` on reads.
It is useful as a placeholder when a `net.Conn` is required but no underlying
connection is available (e.g. in tests or stub implementations).

```go
var conn net.Conn = &onramp.NullConn{}
```

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
