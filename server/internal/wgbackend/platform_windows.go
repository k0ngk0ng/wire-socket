//go:build windows

package wgbackend

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

// setTunAddress sets the IP address on the TUN interface (Windows)
func setTunAddress(name, address string) error {
	// Parse the address
	ip, ipNet, err := net.ParseCIDR(address)
	if err != nil {
		return fmt.Errorf("invalid address %s: %w", address, err)
	}

	mask := ipMaskToStringWin(ipNet.Mask)

	// Use netsh to set the address
	cmd := exec.Command("netsh", "interface", "ip", "set", "address",
		fmt.Sprintf("name=%s", name),
		"source=static",
		fmt.Sprintf("addr=%s", ip.String()),
		fmt.Sprintf("mask=%s", mask))

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set address: %s: %w", string(output), err)
	}

	return nil
}

// setRoutes configures routes through the TUN interface (Windows)
func setRoutes(name string, routes []net.IPNet) error {
	// Get interface index
	// For simplicity, we'll use the route command which accepts interface name
	for _, route := range routes {
		mask := ipMaskToStringWin(route.Mask)
		cmd := exec.Command("route", "add", route.IP.String(), "mask", mask, "0.0.0.0", "if", name)
		if output, err := cmd.CombinedOutput(); err != nil {
			if !strings.Contains(string(output), "already exists") {
				return fmt.Errorf("failed to add route %s: %s: %w", route.String(), string(output), err)
			}
		}
	}
	return nil
}

// ipMaskToStringWin converts an IP mask to dotted decimal notation
func ipMaskToStringWin(mask net.IPMask) string {
	if len(mask) == 4 {
		return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
	}
	return "255.255.255.0" // Default
}
