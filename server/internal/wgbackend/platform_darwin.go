//go:build darwin

package wgbackend

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

// setTunAddress sets the IP address on the TUN interface (macOS)
func setTunAddress(name, address string) error {
	// Parse the address to get the IP
	ip, ipNet, err := net.ParseCIDR(address)
	if err != nil {
		return fmt.Errorf("invalid address %s: %w", address, err)
	}

	// Calculate destination for point-to-point
	// For a /24 network, use the network address as destination
	dest := ipNet.IP.String()

	// Set the address using ifconfig
	cmd := exec.Command("ifconfig", name, "inet", ip.String(), dest, "netmask", ipMaskToString(ipNet.Mask))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set address: %s: %w", string(output), err)
	}

	// Bring interface up
	cmd = exec.Command("ifconfig", name, "up")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring interface up: %s: %w", string(output), err)
	}

	return nil
}

// setRoutes configures routes through the TUN interface (macOS)
func setRoutes(name string, routes []net.IPNet) error {
	for _, route := range routes {
		// Get the gateway (interface address)
		cmd := exec.Command("route", "-n", "add", "-net", route.String(), "-interface", name)
		if output, err := cmd.CombinedOutput(); err != nil {
			// Ignore if route already exists
			if !strings.Contains(string(output), "File exists") && !strings.Contains(string(output), "already in table") {
				return fmt.Errorf("failed to add route %s: %s: %w", route.String(), string(output), err)
			}
		}
	}
	return nil
}

// ipMaskToString converts an IP mask to dotted decimal notation
func ipMaskToString(mask net.IPMask) string {
	if len(mask) == 4 {
		return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
	}
	return "255.255.255.0" // Default
}
