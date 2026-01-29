//go:build !gen
// +build !gen

package onramp

import (
	"os"
	"path/filepath"
	"testing"
)

// TestI2PKeystorePath_EmptyPathReinitializes verifies that when I2P_KEYSTORE_PATH
// is empty, I2PKeystorePath() attempts to reinitialize it.
func TestI2PKeystorePath_EmptyPathReinitializes(t *testing.T) {
	// Save original path to restore after test
	originalPath := I2P_KEYSTORE_PATH
	defer func() { I2P_KEYSTORE_PATH = originalPath }()

	// Set path to empty to simulate failed initialization
	I2P_KEYSTORE_PATH = ""

	// Call the function - it should reinitialize the path
	path, err := I2PKeystorePath()
	if err != nil {
		t.Fatalf("I2PKeystorePath() returned error: %v", err)
	}

	// Verify path is no longer empty
	if path == "" {
		t.Error("I2PKeystorePath() returned empty path after reinitialization")
	}

	// Verify the path ends with the expected directory name
	if filepath.Base(path) != "i2pkeys" {
		t.Errorf("Expected path to end with 'i2pkeys', got: %s", filepath.Base(path))
	}

	// Verify the global variable was updated
	if I2P_KEYSTORE_PATH == "" {
		t.Error("I2P_KEYSTORE_PATH was not updated after reinitialization")
	}
}

// TestTorKeystorePath_EmptyPathReinitializes verifies that when ONION_KEYSTORE_PATH
// is empty, TorKeystorePath() attempts to reinitialize it.
func TestTorKeystorePath_EmptyPathReinitializes(t *testing.T) {
	// Save original path to restore after test
	originalPath := ONION_KEYSTORE_PATH
	defer func() { ONION_KEYSTORE_PATH = originalPath }()

	// Set path to empty to simulate failed initialization
	ONION_KEYSTORE_PATH = ""

	// Call the function - it should reinitialize the path
	path, err := TorKeystorePath()
	if err != nil {
		t.Fatalf("TorKeystorePath() returned error: %v", err)
	}

	// Verify path is no longer empty
	if path == "" {
		t.Error("TorKeystorePath() returned empty path after reinitialization")
	}

	// Verify the path ends with the expected directory name
	if filepath.Base(path) != "onionkeys" {
		t.Errorf("Expected path to end with 'onionkeys', got: %s", filepath.Base(path))
	}

	// Verify the global variable was updated
	if ONION_KEYSTORE_PATH == "" {
		t.Error("ONION_KEYSTORE_PATH was not updated after reinitialization")
	}
}

// TestTLSKeystorePath_EmptyPathReinitializes verifies that when TLS_KEYSTORE_PATH
// is empty, TLSKeystorePath() attempts to reinitialize it.
func TestTLSKeystorePath_EmptyPathReinitializes(t *testing.T) {
	// Save original path to restore after test
	originalPath := TLS_KEYSTORE_PATH
	defer func() { TLS_KEYSTORE_PATH = originalPath }()

	// Set path to empty to simulate failed initialization
	TLS_KEYSTORE_PATH = ""

	// Call the function - it should reinitialize the path
	path, err := TLSKeystorePath()
	if err != nil {
		t.Fatalf("TLSKeystorePath() returned error: %v", err)
	}

	// Verify path is no longer empty
	if path == "" {
		t.Error("TLSKeystorePath() returned empty path after reinitialization")
	}

	// Verify the path ends with the expected directory name
	if filepath.Base(path) != "tlskeys" {
		t.Errorf("Expected path to end with 'tlskeys', got: %s", filepath.Base(path))
	}

	// Verify the global variable was updated
	if TLS_KEYSTORE_PATH == "" {
		t.Error("TLS_KEYSTORE_PATH was not updated after reinitialization")
	}
}

// TestI2PKeystorePath_CreatesDirectory verifies that I2PKeystorePath creates
// the directory if it does not exist.
func TestI2PKeystorePath_CreatesDirectory(t *testing.T) {
	// Save original path to restore after test
	originalPath := I2P_KEYSTORE_PATH
	defer func() { I2P_KEYSTORE_PATH = originalPath }()

	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test-i2pkeys")
	I2P_KEYSTORE_PATH = testPath

	// Verify the directory doesn't exist yet
	if _, err := os.Stat(testPath); !os.IsNotExist(err) {
		t.Fatal("Test directory should not exist before test")
	}

	// Call the function - it should create the directory
	path, err := I2PKeystorePath()
	if err != nil {
		t.Fatalf("I2PKeystorePath() returned error: %v", err)
	}

	// Verify the returned path matches
	if path != testPath {
		t.Errorf("Expected path %s, got %s", testPath, path)
	}

	// Verify the directory was created
	info, err := os.Stat(testPath)
	if err != nil {
		t.Fatalf("Directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Path exists but is not a directory")
	}
}

// TestGetJoinedWD_ReturnsAbsolutePath verifies that GetJoinedWD returns
// an absolute path and creates the directory if needed.
func TestGetJoinedWD_ReturnsAbsolutePath(t *testing.T) {
	path, err := GetJoinedWD("test-keystore-dir")
	if err != nil {
		t.Fatalf("GetJoinedWD() returned error: %v", err)
	}

	// Verify it's an absolute path
	if !filepath.IsAbs(path) {
		t.Errorf("Expected absolute path, got: %s", path)
	}

	// Verify the directory exists
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Path exists but is not a directory")
	}

	// Cleanup
	os.Remove(path)
}
