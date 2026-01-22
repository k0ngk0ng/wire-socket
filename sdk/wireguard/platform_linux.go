// +build linux

package wireguard

import (
	"fmt"
	"log"
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

	log.Printf("Setting TUN address: interface=%s, address=%s", name, address)

	// Set the address using ip command
	cmd := exec.Command("ip", "addr", "add", address, "dev", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		// Check if address already exists
		if !strings.Contains(string(output), "File exists") {
			return fmt.Errorf("failed to add address: %s: %w", string(output), err)
		}
		log.Printf("Address already exists on %s", name)
	} else {
		log.Printf("Address %s added to %s", address, name)
	}

	// Bring interface up
	cmd = exec.Command("ip", "link", "set", name, "up")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring interface up: %s: %w", string(output), err)
	}
	log.Printf("Interface %s is up", name)

	// Verify the route was added (Linux should add it automatically when setting address)
	// If not, add it manually
	ones, _ := ipNet.Mask.Size()
	networkAddr := ip.Mask(ipNet.Mask)
	routeCIDR := fmt.Sprintf("%s/%d", networkAddr.String(), ones)

	cmd = exec.Command("ip", "route", "add", routeCIDR, "dev", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		if !strings.Contains(string(output), "File exists") {
			log.Printf("Note: route add returned: %s", strings.TrimSpace(string(output)))
		}
	} else {
		log.Printf("Route %s added to %s", routeCIDR, name)
	}

	return nil
}

// setRoutes configures routes through the TUN interface (Linux)
func setRoutes(name string, routes []net.IPNet) error {
	for _, route := range routes {
		// Delete any existing route first to ensure it points to the correct interface
		// This is critical for reconnection scenarios where a new TUN interface is created
		deleteCmd := exec.Command("ip", "route", "del", route.String())
		deleteCmd.Run() // Ignore errors - route might not exist

		// Add the route to the new interface
		cmd := exec.Command("ip", "route", "add", route.String(), "dev", name)
		if output, err := cmd.CombinedOutput(); err != nil {
			// Ignore if route already exists (shouldn't happen after delete, but be safe)
			if !strings.Contains(string(output), "File exists") {
				return fmt.Errorf("failed to add route %s: %s: %w", route.String(), string(output), err)
			}
		}
	}
	return nil
}

// cleanupRoutes removes routes on disconnect (no-op on Linux - routes are auto-cleaned)
func cleanupRoutes() {
	// On Linux, routes are automatically removed when the TUN interface is destroyed
}
