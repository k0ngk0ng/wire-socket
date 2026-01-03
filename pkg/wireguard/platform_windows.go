//go:build windows

package wireguard

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"strconv"
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

	log.Printf("Setting TUN address: interface=%s, ip=%s, mask=%s", name, ip.String(), mask)

	// Use netsh to set the address
	cmd := exec.Command("netsh", "interface", "ip", "set", "address",
		fmt.Sprintf("name=%s", name),
		"source=static",
		fmt.Sprintf("addr=%s", ip.String()),
		fmt.Sprintf("mask=%s", mask))

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set address: %s: %w", string(output), err)
	}

	log.Printf("TUN address set successfully")
	return nil
}

// getInterfaceIndex gets the interface index by name using netsh
func getInterfaceIndex(name string) (int, error) {
	// Use netsh to get interface info
	cmd := exec.Command("netsh", "interface", "ipv4", "show", "interfaces")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed to list interfaces: %w", err)
	}

	// Parse output to find interface index
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, name) {
			fields := strings.Fields(line)
			if len(fields) >= 1 {
				idx, err := strconv.Atoi(fields[0])
				if err == nil {
					return idx, nil
				}
			}
		}
	}

	return 0, fmt.Errorf("interface %s not found", name)
}

// setRoutes configures routes through the TUN interface (Windows)
func setRoutes(name string, routes []net.IPNet) error {
	// Get the interface index
	ifIndex, err := getInterfaceIndex(name)
	if err != nil {
		log.Printf("Warning: could not get interface index for %s: %v", name, err)
		// Try using netsh as fallback
		return setRoutesNetsh(name, routes)
	}

	log.Printf("Got interface index %d for %s", ifIndex, name)

	for _, route := range routes {
		mask := ipMaskToStringWin(route.Mask)

		// Use route add with interface index
		// Syntax: route add <destination> mask <netmask> <gateway> if <interface_index>
		// For TUN, we use 0.0.0.0 as gateway and specify the interface
		cmd := exec.Command("route", "add", route.IP.String(), "mask", mask, "0.0.0.0", "if", strconv.Itoa(ifIndex))
		output, err := cmd.CombinedOutput()
		if err != nil {
			outputStr := string(output)
			if !strings.Contains(outputStr, "already exists") && !strings.Contains(outputStr, "object already exists") {
				log.Printf("Warning: failed to add route %s via route command: %s", route.String(), outputStr)
				// Try netsh as fallback
				if err := addRouteNetsh(name, route); err != nil {
					log.Printf("Warning: failed to add route %s via netsh: %v", route.String(), err)
				}
			}
		} else {
			log.Printf("Added route %s via interface %d", route.String(), ifIndex)
		}
	}
	return nil
}

// setRoutesNetsh uses netsh to set routes (fallback method)
func setRoutesNetsh(name string, routes []net.IPNet) error {
	for _, route := range routes {
		if err := addRouteNetsh(name, route); err != nil {
			log.Printf("Warning: failed to add route %s: %v", route.String(), err)
		}
	}
	return nil
}

// addRouteNetsh adds a single route using netsh
func addRouteNetsh(name string, route net.IPNet) error {
	ones, _ := route.Mask.Size()
	prefix := fmt.Sprintf("%s/%d", route.IP.String(), ones)

	cmd := exec.Command("netsh", "interface", "ipv4", "add", "route",
		prefix,
		fmt.Sprintf("interface=%s", name),
		"store=active")

	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		if strings.Contains(outputStr, "already exists") || strings.Contains(outputStr, "object already exists") {
			log.Printf("Route %s already exists", prefix)
			return nil
		}
		return fmt.Errorf("netsh add route failed: %s: %w", outputStr, err)
	}

	log.Printf("Added route %s via netsh", prefix)
	return nil
}

// ipMaskToStringWin converts an IP mask to dotted decimal notation
func ipMaskToStringWin(mask net.IPMask) string {
	if len(mask) == 4 {
		return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
	}
	return "255.255.255.0" // Default
}
