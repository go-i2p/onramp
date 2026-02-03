Example Usage
=============

### Usage as instance of a struct, Listener

```Go

package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/go-i2p/onramp"
)

func main() {
	garlic, err := onramp.NewGarlic("my-listener", onramp.SAM_ADDR, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer garlic.Close()
	listener, err := garlic.Listen()
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello, %q", r.URL.Path)
	})
	if err := http.Serve(listener, nil); err != nil {
		log.Fatal(err)
	}
}
```

### Usage as instance of a struct, Dialer

```Go

package main

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/go-i2p/onramp"
)

func main() {
	garlic, err := onramp.NewGarlic("my-dialer", onramp.SAM_ADDR, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer garlic.Close()
	transport := http.Transport{
		Dial: garlic.Dial,
	}
	client := &http.Client{
		Transport: &transport,
	}
	// Replace with an actual I2P destination address
	resp, err := client.Get("http://example.i2p/")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	fmt.Println(resp.Status)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(body))
}

```

### Usage as instance of a struct, Listener and Dialer on same address

```Go

package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"

	"github.com/go-i2p/onramp"
)

func main() {
	garlic, err := onramp.NewGarlic("my-bidirectional", onramp.SAM_ADDR, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer garlic.Close()
	listener, err := garlic.Listen()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("listener:", listener.Addr().String())
	defer listener.Close()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello, %q", r.URL.Path)
	})
	go Serve(listener)
	transport := http.Transport{
		Dial: garlic.Dial,
	}
	client := &http.Client{
		Transport: &transport,
	}
	resp, err := client.Get("http://" + listener.Addr().String() + "/")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	fmt.Println(resp.Status)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(body))
}

func Serve(listener net.Listener) {
	if err := http.Serve(listener, nil); err != nil {
		log.Fatal(err)
	}
}
```

### Usage as automatically-managed Listeners

```Go

package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/go-i2p/onramp"
)

func main() {
	defer onramp.CloseAll()
	listener, err := onramp.Listen("tcp", "service.i2p")
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello, %q", r.URL.Path)
	})
	if err := http.Serve(listener, nil); err != nil {
		log.Fatal(err)
	}
}

```

### Usage as automatically-managed Dialers

```Go

package main

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/go-i2p/onramp"
)

func main() {
	defer onramp.CloseAll()
	transport := http.Transport{
		Dial: onramp.Dial,
	}
	client := &http.Client{
		Transport: &transport,
	}
	// Replace with an actual I2P destination address
	resp, err := client.Get("http://example.i2p/")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	fmt.Println(resp.Status)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(body))
}

```
