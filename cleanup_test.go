//go:build !gen
// +build !gen

package onramp

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestCleanupRegistry(t *testing.T) {
	// Create a mock Garlic-like struct for testing
	// We can't easily test the actual Garlic cleanup without a SAM bridge,
	// but we can test the registry mechanics

	t.Run("register and unregister", func(t *testing.T) {
		// Verify initial state
		registry.mu.Lock()
		initialCount := len(registry.instances)
		registry.mu.Unlock()

		// Create a minimal Garlic struct for testing
		g := &Garlic{name: "test-cleanup"}

		// Register it
		registry.register(g)

		registry.mu.Lock()
		afterRegister := len(registry.instances)
		registry.mu.Unlock()

		if afterRegister != initialCount+1 {
			t.Errorf("Expected %d instances after register, got %d", initialCount+1, afterRegister)
		}

		// Unregister it
		registry.unregister(g)

		registry.mu.Lock()
		afterUnregister := len(registry.instances)
		registry.mu.Unlock()

		if afterUnregister != initialCount {
			t.Errorf("Expected %d instances after unregister, got %d", initialCount, afterUnregister)
		}
	})

	t.Run("double register is idempotent", func(t *testing.T) {
		g := &Garlic{name: "test-double-register"}

		registry.register(g)
		registry.mu.Lock()
		count1 := len(registry.instances)
		registry.mu.Unlock()

		registry.register(g) // Register again
		registry.mu.Lock()
		count2 := len(registry.instances)
		registry.mu.Unlock()

		// Should still be the same count since we use a map
		if count1 != count2 {
			t.Errorf("Double register changed count from %d to %d", count1, count2)
		}

		// Cleanup
		registry.unregister(g)
	})

	t.Run("unregister non-existent is safe", func(t *testing.T) {
		g := &Garlic{name: "never-registered"}

		// Should not panic
		registry.unregister(g)
	})
}

func TestEnableAutoCleanup(t *testing.T) {
	t.Run("nil garlic is safe", func(t *testing.T) {
		// Should not panic
		EnableAutoCleanup(nil)
		DisableAutoCleanup(nil)
	})

	t.Run("enable and disable", func(t *testing.T) {
		g := &Garlic{name: "test-auto-cleanup"}

		registry.mu.Lock()
		initialCount := len(registry.instances)
		registry.mu.Unlock()

		EnableAutoCleanup(g)

		registry.mu.Lock()
		afterEnable := len(registry.instances)
		registry.mu.Unlock()

		if afterEnable != initialCount+1 {
			t.Errorf("Expected %d instances after enable, got %d", initialCount+1, afterEnable)
		}

		DisableAutoCleanup(g)

		registry.mu.Lock()
		afterDisable := len(registry.instances)
		registry.mu.Unlock()

		if afterDisable != initialCount {
			t.Errorf("Expected %d instances after disable, got %d", initialCount, afterDisable)
		}
	})
}

func TestCleanupHooks(t *testing.T) {
	t.Run("hook execution", func(t *testing.T) {
		var mu sync.Mutex
		executed := false

		RegisterCleanupHook(func() {
			mu.Lock()
			executed = true
			mu.Unlock()
		})

		runCleanupHooks()

		mu.Lock()
		if !executed {
			t.Error("Cleanup hook was not executed")
		}
		mu.Unlock()
	})

	t.Run("panic recovery in hooks", func(t *testing.T) {
		// Register a hook that panics
		RegisterCleanupHook(func() {
			panic("test panic")
		})

		// Should not panic
		runCleanupHooks()
	})
}

func TestFinalizerSetup(t *testing.T) {
	t.Run("finalizer is set", func(t *testing.T) {
		g := &Garlic{name: "test-finalizer"}

		setupFinalizer(g)

		// We can't easily test that the finalizer runs, but we can
		// verify the setup doesn't panic and we can clear it
		runtime.SetFinalizer(g, nil)
	})
}

func TestCleanupConcurrency(t *testing.T) {
	t.Run("concurrent register and unregister", func(t *testing.T) {
		var wg sync.WaitGroup
		instances := make([]*Garlic, 100)

		for i := 0; i < 100; i++ {
			instances[i] = &Garlic{name: "test-concurrent-" + string(rune('0'+i))}
		}

		// Concurrently register all
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(g *Garlic) {
				defer wg.Done()
				registry.register(g)
			}(instances[i])
		}
		wg.Wait()

		// Concurrently unregister all
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(g *Garlic) {
				defer wg.Done()
				time.Sleep(time.Millisecond) // Small delay to encourage race conditions
				registry.unregister(g)
			}(instances[i])
		}
		wg.Wait()
	})
}
