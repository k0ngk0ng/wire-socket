// Package route provides IP routing management for VPN traffic
package route

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
)

// Route represents a routing rule
type Route struct {
	CIDR    string // Destination CIDR (e.g., "192.168.1.0/24")
	Gateway string // Next hop (optional)
	Device  string // Interface (optional)
	Metric  int    // Route priority (optional, lower = higher priority)
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
		if runtime.GOOS == "darwin" {
			// macOS: replace "add" with "delete"
			deleteRule := strings.Replace(rule, " add ", " delete ", 1)
			args := strings.Fields(deleteRule)
			if len(args) > 0 {
				cmd := exec.Command("route", args...)
				if output, err := cmd.CombinedOutput(); err != nil {
					log.Printf("Warning: failed to remove route: %s: %v", strings.TrimSpace(string(output)), err)
				}
			}
		} else {
			// Linux: replace "add" with "del"
			deleteRule := strings.Replace(rule, " add ", " del ", 1)
			args := strings.Fields(deleteRule)
			if len(args) > 0 {
				cmd := exec.Command("ip", args...)
				if output, err := cmd.CombinedOutput(); err != nil {
					log.Printf("Warning: failed to remove route: %s: %v", strings.TrimSpace(string(output)), err)
				}
			}
		}
	}

	m.appliedRules = []string{}
	log.Println("Routes cleaned up")
}

// addRoute adds a single route
func (m *Manager) addRoute(route Route) error {
	device := route.Device
	if device == "" {
		device = m.config.DefaultDevice
	}

	if runtime.GOOS == "darwin" {
		return m.addRouteDarwin(route, device)
	}
	return m.addRouteLinux(route, device)
}

// addRouteLinux adds a route on Linux using ip command
func (m *Manager) addRouteLinux(route Route, device string) error {
	// Build the ip route command
	// ip route add 192.168.1.0/24 via 10.0.0.1 dev wg0 metric 100
	var ruleArgs []string
	ruleArgs = append(ruleArgs, "route", "add", route.CIDR)

	if route.Gateway != "" {
		ruleArgs = append(ruleArgs, "via", route.Gateway)
	}

	if device != "" {
		ruleArgs = append(ruleArgs, "dev", device)
	}

	if route.Metric > 0 {
		ruleArgs = append(ruleArgs, "metric", fmt.Sprintf("%d", route.Metric))
	}

	ruleStr := strings.Join(ruleArgs, " ")

	// Check if route already exists
	checkCmd := exec.Command("ip", "route", "show", route.CIDR)
	if output, _ := checkCmd.Output(); len(strings.TrimSpace(string(output))) > 0 {
		log.Printf("Route %s already exists, skipping", route.CIDR)
		return nil
	}

	// Apply the route
	cmd := exec.Command("ip", ruleArgs...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}

	m.appliedRules = append(m.appliedRules, ruleStr)
	m.logAppliedRoute(route, device)
	return nil
}

// addRouteDarwin adds a route on macOS using route command
func (m *Manager) addRouteDarwin(route Route, device string) error {
	// Build the route command for macOS
	// route -n add -net 192.168.1.0/24 10.0.0.1
	// or: route -n add -net 192.168.1.0/24 -interface utun0
	var ruleArgs []string
	ruleArgs = append(ruleArgs, "-n", "add", "-net", route.CIDR)

	if route.Gateway != "" {
		ruleArgs = append(ruleArgs, route.Gateway)
	} else if device != "" {
		ruleArgs = append(ruleArgs, "-interface", device)
	}

	ruleStr := strings.Join(ruleArgs, " ")

	// Check if route already exists
	checkCmd := exec.Command("netstat", "-rn")
	if output, err := checkCmd.Output(); err == nil {
		if strings.Contains(string(output), route.CIDR) {
			log.Printf("Route %s already exists, skipping", route.CIDR)
			return nil
		}
	}

	// Apply the route
	cmd := exec.Command("route", ruleArgs...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}

	m.appliedRules = append(m.appliedRules, ruleStr)
	m.logAppliedRoute(route, device)
	return nil
}

// logAppliedRoute logs the applied route
func (m *Manager) logAppliedRoute(route Route, device string) {
	var parts []string
	parts = append(parts, route.CIDR)
	if route.Gateway != "" {
		parts = append(parts, fmt.Sprintf("via %s", route.Gateway))
	}
	if device != "" {
		parts = append(parts, fmt.Sprintf("dev %s", device))
	}
	if route.Metric > 0 {
		parts = append(parts, fmt.Sprintf("metric %d", route.Metric))
	}
	log.Printf("Applied route: %s", strings.Join(parts, " "))
}

// GetAppliedCount returns the number of applied routes
func (m *Manager) GetAppliedCount() int {
	return len(m.appliedRules)
}
