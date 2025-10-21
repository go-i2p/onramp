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
	defer garlic.Close()
	listener, err := onion.Listen()
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
}
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
