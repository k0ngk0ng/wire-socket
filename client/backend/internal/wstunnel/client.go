package wstunnel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Client manages the wstunnel client process
type Client struct {
	serverAddr string
	localPort  int
	remotePort int
	cmd        *exec.Cmd
}

// NewClient creates a new wstunnel client
func NewClient(serverAddr string, remotePort int) (*Client, error) {
	return &Client{
		serverAddr: serverAddr,
		localPort:  51820, // Local port to listen on
		remotePort: remotePort,
	}, nil
}

// Start starts the wstunnel client process
func (c *Client) Start() error {
	// Find wstunnel binary
	wstunnelPath, err := findWSTunnelBinary()
	if err != nil {
		return fmt.Errorf("wstunnel binary not found: %w", err)
	}

	// Build wstunnel command
	// wstunnel client -L udp://127.0.0.1:51820:127.0.0.1:51820 wss://server:443
	localEndpoint := fmt.Sprintf("udp://127.0.0.1:%d:127.0.0.1:%d", c.localPort, c.remotePort)
	serverURL := fmt.Sprintf("wss://%s:443", c.serverAddr)

	// If server address doesn't start with wss:// or ws://, prepend it
	if c.serverAddr[:2] != "ws" {
		// Default to wss for security
		serverURL = fmt.Sprintf("wss://%s:443", c.serverAddr)
	}

	c.cmd = exec.Command(wstunnelPath, "client", "-L", localEndpoint, serverURL)
	c.cmd.Stdout = os.Stdout
	c.cmd.Stderr = os.Stderr

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start wstunnel: %w", err)
	}

	fmt.Printf("wstunnel client started (PID: %d)\n", c.cmd.Process.Pid)
	return nil
}

// Stop stops the wstunnel client process
func (c *Client) Stop() error {
	if c.cmd == nil || c.cmd.Process == nil {
		return nil
	}

	if err := c.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to kill wstunnel process: %w", err)
	}

	// Wait for process to exit
	c.cmd.Wait()

	fmt.Println("wstunnel client stopped")
	return nil
}

// IsRunning checks if the wstunnel process is running
func (c *Client) IsRunning() bool {
	if c.cmd == nil || c.cmd.Process == nil {
		return false
	}

	// Check if process has exited
	return c.cmd.ProcessState == nil || !c.cmd.ProcessState.Exited()
}

// findWSTunnelBinary finds the wstunnel binary path
func findWSTunnelBinary() (string, error) {
	// Get the executable path
	exePath, err := os.Executable()
	if err == nil {
		// Try to find wstunnel in the same directory as the executable (bundled)
		exeDir := filepath.Dir(exePath)
		var bundledPath string

		if runtime.GOOS == "windows" {
			bundledPath = filepath.Join(exeDir, "wstunnel.exe")
		} else {
			bundledPath = filepath.Join(exeDir, "wstunnel")
		}

		if _, err := os.Stat(bundledPath); err == nil {
			return bundledPath, nil
		}

		// For macOS app bundle, check Resources directory
		if runtime.GOOS == "darwin" {
			// Check if we're running from an app bundle
			// Path might be: /Applications/WireSocket.app/Contents/Resources/bin/wire-socket-client
			resourcesPath := filepath.Join(exeDir, "wstunnel")
			if _, err := os.Stat(resourcesPath); err == nil {
				return resourcesPath, nil
			}
		}
	}

	// Try to find wstunnel in PATH
	path, err := exec.LookPath("wstunnel")
	if err == nil {
		return path, nil
	}

	// Try common installation locations
	commonPaths := []string{
		"/usr/local/bin/wstunnel",
		"/usr/bin/wstunnel",
	}

	if runtime.GOOS == "windows" {
		commonPaths = []string{
			"C:\\Program Files\\wstunnel\\wstunnel.exe",
			"wstunnel.exe",
		}
	}

	for _, p := range commonPaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("wstunnel binary not found in PATH or common locations")
}
