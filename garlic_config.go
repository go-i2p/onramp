package onramp

import sam3 "github.com/go-i2p/go-sam-go"

// SAM tunnel configuration options.
// These constants control the number of tunnels created for I2P connections.
// More tunnels provide greater anonymity and redundancy but consume more resources.
//
// Usage: Pass to NewGarlic() as the options parameter.
//
//	garlic, err := onramp.NewGarlic("myservice", onramp.SAM_ADDR, onramp.OPT_WIDE)
var (
	// OPT_DEFAULTS uses the SAM library's default tunnel configuration.
	// This is a balanced configuration suitable for most applications.
	// Typically creates 2-3 inbound and outbound tunnels of length 3.
	OPT_DEFAULTS = sam3.Options_Default

	// OPT_WIDE creates more parallel tunnels for increased redundancy
	// and improved latency through path diversity. Uses more bandwidth
	// and connections but provides better performance for high-traffic services.
	OPT_WIDE = sam3.Options_Wide
)

// Tunnel quantity presets for different use cases.
// Use smaller configurations for client applications that need fewer resources,
// and larger configurations for servers expecting high traffic.
var (
	// OPT_HUGE (Humongous) creates the maximum number of tunnels.
	// Best for high-traffic servers requiring maximum redundancy.
	// Warning: Consumes significant bandwidth and router resources.
	OPT_HUGE = sam3.Options_Humongous

	// OPT_LARGE creates many tunnels for busy services.
	// Good balance of redundancy and resource usage for active servers.
	OPT_LARGE = sam3.Options_Large

	// OPT_MEDIUM creates a moderate number of tunnels.
	// Suitable for services with moderate traffic expectations.
	OPT_MEDIUM = sam3.Options_Medium

	// OPT_SMALL creates minimal tunnels to conserve resources.
	// Best for client applications or low-traffic services.
	// Provides less redundancy but uses minimal router resources.
	OPT_SMALL = sam3.Options_Small
)
