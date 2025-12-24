// +build linux

package wireguard

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

// setTunAddress sets the IP address on the TUN interface (Linux)
func setTunAddress(name, address string) error {
	// Parse the address to get the IP and prefix
	ip, ipNet, err := net.ParseCIDR(address)
	if err != nil {
		return fmt.Errorf("invalid address %s: %w", address, err)
	}

	// Set the address using ip command
	cmd := exec.Command("ip", "addr", "add", address, "dev", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		// Check if address already exists
		if !strings.Contains(string(output), "File exists") {
			return fmt.Errorf("failed to add address: %s: %w", string(output), err)
		}
	}

	// Bring interface up
	cmd = exec.Command("ip", "link", "set", name, "up")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring interface up: %s: %w", string(output), err)
	}

	_ = ip
	_ = ipNet

	return nil
}

// setRoutes configures routes through the TUN interface (Linux)
func setRoutes(name string, routes []net.IPNet) error {
	for _, route := range routes {
		cmd := exec.Command("ip", "route", "add", route.String(), "dev", name)
		if output, err := cmd.CombinedOutput(); err != nil {
			// Ignore if route already exists
			if !strings.Contains(string(output), "File exists") {
				return fmt.Errorf("failed to add route %s: %s: %w", route.String(), string(output), err)
			}
		}
	}
	return nil
}
