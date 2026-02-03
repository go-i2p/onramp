//go:build !gen
// +build !gen

package onramp

import (
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
)

// cleanupRegistry maintains a registry of Garlic instances that need
// cleanup when the program exits. This provides guaranteed cleanup
// even when downstream consumers fail to call Close().
type cleanupRegistry struct {
	mu        sync.Mutex
	instances map[*Garlic]struct{}
	once      sync.Once
	sigChan   chan os.Signal
	doneChan  chan struct{}
}

var registry = &cleanupRegistry{
	instances: make(map[*Garlic]struct{}),
	sigChan:   make(chan os.Signal, 1),
	doneChan:  make(chan struct{}),
}

// init sets up signal handlers to catch SIGINT, SIGTERM, and other
// termination signals so we can clean up SAM sessions before exit.
func init() {
	registry.once.Do(func() {
		signal.Notify(registry.sigChan,
			syscall.SIGINT,
			syscall.SIGTERM,
			syscall.SIGHUP,
		)
		go registry.signalHandler()
	})
}

// signalHandler listens for termination signals and runs cleanup.
func (r *cleanupRegistry) signalHandler() {
	sig := <-r.sigChan
	log.WithField("signal", sig).Info("Received termination signal, cleaning up SAM sessions")
	r.cleanupAll()
	close(r.doneChan)
	// Re-raise the signal with default handler for proper exit
	signal.Reset(sig.(syscall.Signal))
	p, _ := os.FindProcess(os.Getpid())
	p.Signal(sig)
}

// register adds a Garlic instance to the cleanup registry.
// This should be called when a new Garlic is created.
func (r *cleanupRegistry) register(g *Garlic) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.instances[g] = struct{}{}
	log.WithField("name", g.getName()).Debug("Registered Garlic instance for cleanup")
}

// unregister removes a Garlic instance from the cleanup registry.
// This should be called when Close() is explicitly called.
func (r *cleanupRegistry) unregister(g *Garlic) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.instances, g)
	log.WithField("name", g.getName()).Debug("Unregistered Garlic instance from cleanup")
}

// cleanupAll closes all registered Garlic instances and runs cleanup hooks.
// This is called when a termination signal is received.
func (r *cleanupRegistry) cleanupAll() {
	// Run cleanup hooks first
	runCleanupHooks()

	r.mu.Lock()
	instances := make([]*Garlic, 0, len(r.instances))
	for g := range r.instances {
		instances = append(instances, g)
	}
	r.mu.Unlock()

	log.WithField("count", len(instances)).Info("Cleaning up registered SAM sessions")
	for _, g := range instances {
		log.WithField("name", g.getName()).Debug("Cleaning up Garlic instance")
		if err := g.Close(); err != nil {
			log.WithError(err).WithField("name", g.getName()).Error("Error during cleanup")
		}
	}
}

// setupFinalizer sets up a runtime finalizer for the Garlic instance.
// The finalizer will attempt to close the session if it hasn't been closed
// already when the Garlic object is garbage collected.
//
// Note: Finalizers are not guaranteed to run (e.g., on os.Exit or crashes),
// so this is a best-effort fallback. Signal handlers provide more reliable
// cleanup for graceful termination.
func setupFinalizer(g *Garlic) {
	runtime.SetFinalizer(g, func(g *Garlic) {
		// Check if already closed by checking if sessions exist
		if g.primary != nil || g.SAM != nil || g.streamSub != nil || g.hybrid2Sub != nil {
			log.WithField("name", g.getName()).Warn("Garlic instance garbage collected without Close(), attempting cleanup")
			if err := g.Close(); err != nil {
				log.WithError(err).WithField("name", g.getName()).Error("Finalizer cleanup failed")
			}
		}
		// Unregister from cleanup registry
		registry.unregister(g)
	})
}

// EnableAutoCleanup registers a Garlic instance for automatic cleanup
// on program termination. This includes:
// 1. Signal handler cleanup (SIGINT, SIGTERM, SIGHUP)
// 2. Runtime finalizer cleanup (garbage collection)
//
// This function should be called after creating a Garlic instance if you
// want guaranteed cleanup even when Close() is not explicitly called.
//
// Example usage:
//
//	g, err := onramp.NewGarlic("myapp", onramp.SAM_ADDR, onramp.OPT_DEFAULTS)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	onramp.EnableAutoCleanup(g)
//	defer g.Close() // Still recommended, but cleanup is now guaranteed
func EnableAutoCleanup(g *Garlic) {
	if g == nil {
		return
	}
	registry.register(g)
	setupFinalizer(g)
}

// DisableAutoCleanup removes a Garlic instance from automatic cleanup.
// This should be called if you want to manage the lifecycle manually
// after having called EnableAutoCleanup.
func DisableAutoCleanup(g *Garlic) {
	if g == nil {
		return
	}
	registry.unregister(g)
	// Clear the finalizer
	runtime.SetFinalizer(g, nil)
}

// RegisterCleanupHook allows applications to register additional cleanup
// functions that will be called during signal-based cleanup.
// This is useful for cleaning up related resources.
var (
	cleanupHooks   []func()
	cleanupHooksMu sync.Mutex
)

// RegisterCleanupHook adds a function to be called during cleanup.
// Hooks are called in the order they were registered.
func RegisterCleanupHook(hook func()) {
	cleanupHooksMu.Lock()
	defer cleanupHooksMu.Unlock()
	cleanupHooks = append(cleanupHooks, hook)
}

// runCleanupHooks executes all registered cleanup hooks.
func runCleanupHooks() {
	cleanupHooksMu.Lock()
	hooks := make([]func(), len(cleanupHooks))
	copy(hooks, cleanupHooks)
	cleanupHooksMu.Unlock()

	for _, hook := range hooks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.WithField("panic", r).Error("Cleanup hook panicked")
				}
			}()
			hook()
		}()
	}
}
