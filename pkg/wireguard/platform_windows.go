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

// tunInterfaceIP stores the TUN interface IP for routing decisions
var tunInterfaceIP net.IP

// tunGatewayIP stores the VPN gateway IP (e.g., 10.250.99.1)
var tunGatewayIP net.IP

// setTunAddress sets the IP address on the TUN interface (Windows)
func setTunAddress(name, address string) error {
	// Parse the address
	ip, _, err := net.ParseCIDR(address)
	if err != nil {
		return fmt.Errorf("invalid address %s: %w", address, err)
	}

	// Ensure we're working with IPv4
	ip4 := ip.To4()
	if ip4 == nil {
		return fmt.Errorf("only IPv4 addresses are supported: %s", address)
	}

	// For Windows, we use /24 subnet mask similar to macOS
	mask := "255.255.255.0"

	// Calculate the gateway IP (first IP in subnet, like 10.250.99.1)
	// For IP 10.250.99.4, gateway is 10.250.99.1
	gatewayIP := make(net.IP, 4)
	copy(gatewayIP, ip4)
	gatewayIP[3] = 1

	log.Printf("Setting TUN address: interface=%s, ip=%s, mask=%s, gateway=%s", name, ip4.String(), mask, gatewayIP.String())

	// Store the IP and gateway for routing decisions
	tunInterfaceIP = ip4
	tunGatewayIP = gatewayIP

	// Use netsh to set the address with /24 mask
	// First try without gateway (gateway often fails on TUN interfaces)
	cmd := exec.Command("netsh", "interface", "ip", "set", "address",
		fmt.Sprintf("name=%s", name),
		"source=static",
		fmt.Sprintf("addr=%s", ip4.String()),
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

	// Use the VPN gateway (e.g., 10.250.99.1) as next hop
	gatewayIP := ""
	if tunGatewayIP != nil {
		gatewayIP = tunGatewayIP.String()
	}

	for _, route := range routes {
		mask := ipMaskToStringWin(route.Mask)

		// Use the VPN gateway as next hop for all routes
		gateway := gatewayIP
		if gateway == "" {
			gateway = "0.0.0.0"
		}

		cmd := exec.Command("route", "add", route.IP.String(), "mask", mask, gateway, "if", strconv.Itoa(ifIndex))
		log.Printf("Executing: route add %s mask %s %s if %d", route.IP.String(), mask, gateway, ifIndex)
		output, err := cmd.CombinedOutput()
		if err != nil {
			outputStr := string(output)
			if !strings.Contains(outputStr, "already exists") && !strings.Contains(outputStr, "object already exists") {
				log.Printf("Warning: failed to add route %s via route command: %s", route.String(), outputStr)
				// Try netsh as fallback
				if err := addRouteNetsh(name, route, gatewayIP); err != nil {
					log.Printf("Warning: failed to add route %s via netsh: %v", route.String(), err)
				}
			}
		} else {
			log.Printf("Added route %s (gateway %s) via interface %d", route.String(), gateway, ifIndex)
		}
	}
	return nil
}

// setRoutesNetsh uses netsh to set routes (fallback method)
func setRoutesNetsh(name string, routes []net.IPNet) error {
	gatewayIP := ""
	if tunGatewayIP != nil {
		gatewayIP = tunGatewayIP.String()
	}

	for _, route := range routes {
		if err := addRouteNetsh(name, route, gatewayIP); err != nil {
			log.Printf("Warning: failed to add route %s: %v", route.String(), err)
		}
	}
	return nil
}

// addRouteNetsh adds a single route using netsh
func addRouteNetsh(name string, route net.IPNet, gatewayIP string) error {
	ones, _ := route.Mask.Size()
	prefix := fmt.Sprintf("%s/%d", route.IP.String(), ones)

	var cmd *exec.Cmd
	if gatewayIP != "" {
		cmd = exec.Command("netsh", "interface", "ipv4", "add", "route",
			prefix,
			fmt.Sprintf("interface=%s", name),
			fmt.Sprintf("nexthop=%s", gatewayIP),
			"store=active")
	} else {
		cmd = exec.Command("netsh", "interface", "ipv4", "add", "route",
			prefix,
			fmt.Sprintf("interface=%s", name),
			"store=active")
	}

	log.Printf("Executing netsh: %v", cmd.Args)
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
	// Handle 16-byte masks (IPv4 in IPv6 format)
	if len(mask) == 16 {
		return fmt.Sprintf("%d.%d.%d.%d", mask[12], mask[13], mask[14], mask[15])
	}
	return "255.255.255.0" // Default
}
