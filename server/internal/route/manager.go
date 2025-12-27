// Package route provides IP routing management for VPN traffic
package route

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// Route represents a routing rule
type Route struct {
	CIDR    string // Destination CIDR (e.g., "192.168.1.0/24")
	Gateway string // Next hop (optional)
	Device  string // Interface (optional)
}

// Config holds route configuration
type Config struct {
	DefaultDevice string  // Default device for routes without explicit device
	Routes        []Route // Routes to apply
}

// Manager manages IP routes
type Manager struct {
	config       Config
	appliedRules []string // Track applied routes for cleanup
}

// NewManager creates a new route manager
func NewManager(cfg Config) *Manager {
	return &Manager{
		config:       cfg,
		appliedRules: []string{},
	}
}

// Apply adds all routes to the system routing table
func (m *Manager) Apply() error {
	for _, route := range m.config.Routes {
		if err := m.addRoute(route); err != nil {
			log.Printf("Warning: failed to add route %s: %v", route.CIDR, err)
		}
	}

	log.Printf("Routes applied: %d routes", len(m.config.Routes))
	return nil
}

// Cleanup removes all applied routes
func (m *Manager) Cleanup() {
	log.Println("Cleaning up routes...")

	for i := len(m.appliedRules) - 1; i >= 0; i-- {
		rule := m.appliedRules[i]
		// Replace "add" with "del"
		deleteRule := strings.Replace(rule, " add ", " del ", 1)
		args := strings.Fields(deleteRule)
		if len(args) > 0 {
			cmd := exec.Command("ip", args...)
			if output, err := cmd.CombinedOutput(); err != nil {
				log.Printf("Warning: failed to remove route: %s: %v", strings.TrimSpace(string(output)), err)
			}
		}
	}

	m.appliedRules = []string{}
	log.Println("Routes cleaned up")
}

// addRoute adds a single route
func (m *Manager) addRoute(route Route) error {
	// Build the ip route command
	// ip route add 192.168.1.0/24 via 10.0.0.1 dev wg0
	// or just: ip route add 192.168.1.0/24 dev wg0

	var ruleArgs []string
	ruleArgs = append(ruleArgs, "route", "add", route.CIDR)

	if route.Gateway != "" {
		ruleArgs = append(ruleArgs, "via", route.Gateway)
	}

	device := route.Device
	if device == "" {
		device = m.config.DefaultDevice
	}
	if device != "" {
		ruleArgs = append(ruleArgs, "dev", device)
	}

	ruleStr := strings.Join(ruleArgs, " ")

	// Check if route already exists
	checkArgs := make([]string, len(ruleArgs))
	copy(checkArgs, ruleArgs)
	checkArgs[1] = "show" // Change "add" to "show"
	checkArgs = checkArgs[:3] // Just "route show CIDR"

	checkCmd := exec.Command("ip", checkArgs...)
	if output, _ := checkCmd.Output(); len(strings.TrimSpace(string(output))) > 0 {
		log.Printf("Route %s already exists", route.CIDR)
		return nil
	}

	// Apply the route
	cmd := exec.Command("ip", ruleArgs...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}

	m.appliedRules = append(m.appliedRules, ruleStr)

	if route.Gateway != "" {
		log.Printf("Applied route: %s via %s dev %s", route.CIDR, route.Gateway, device)
	} else {
		log.Printf("Applied route: %s dev %s", route.CIDR, device)
	}

	return nil
}

// GetAppliedCount returns the number of applied routes
func (m *Manager) GetAppliedCount() int {
	return len(m.appliedRules)
}
