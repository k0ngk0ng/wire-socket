// Package nat provides NAT/iptables management for VPN traffic forwarding
package nat

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// MasqueradeRule represents a MASQUERADE rule
type MasqueradeRule struct {
	Interface string // Outbound interface (e.g., "eth0")
}

// SNATRule represents a Source NAT rule
type SNATRule struct {
	Source      string // Source CIDR (e.g., "10.250.2.0/24")
	Destination string // Destination CIDR (e.g., "192.168.250.0/24")
	Interface   string // Outbound interface (e.g., "wg0")
	ToSource    string // SNAT to this IP (e.g., "192.168.250.8")
}

// DNATRule represents a Destination NAT (port forwarding) rule
type DNATRule struct {
	Interface     string // Incoming interface (e.g., "eth0")
	Protocol      string // tcp or udp
	Port          int    // External port
	ToDestination string // Forward to this address:port (e.g., "10.250.2.5:80")
}

// Config holds NAT configuration
type Config struct {
	Enabled    bool
	Masquerade []MasqueradeRule
	SNAT       []SNATRule
	DNAT       []DNATRule
}

// Manager manages iptables NAT rules
type Manager struct {
	config       Config
	appliedRules []string // Track applied rules for cleanup
}

// NewManager creates a new NAT manager
func NewManager(cfg Config) *Manager {
	return &Manager{
		config:       cfg,
		appliedRules: []string{},
	}
}

// Apply enables IP forwarding and applies all NAT rules
func (m *Manager) Apply() error {
	if !m.config.Enabled {
		log.Println("NAT is disabled, skipping")
		return nil
	}

	// Enable IP forwarding
	if err := m.enableIPForwarding(); err != nil {
		return fmt.Errorf("failed to enable IP forwarding: %w", err)
	}

	// Apply MASQUERADE rules
	for _, rule := range m.config.Masquerade {
		if err := m.applyMasquerade(rule); err != nil {
			log.Printf("Warning: failed to apply masquerade rule for %s: %v", rule.Interface, err)
		}
	}

	// Apply SNAT rules
	for _, rule := range m.config.SNAT {
		if err := m.applySNAT(rule); err != nil {
			log.Printf("Warning: failed to apply SNAT rule: %v", err)
		}
	}

	// Apply DNAT rules
	for _, rule := range m.config.DNAT {
		if err := m.applyDNAT(rule); err != nil {
			log.Printf("Warning: failed to apply DNAT rule: %v", err)
		}
	}

	log.Printf("NAT rules applied: %d masquerade, %d SNAT, %d DNAT",
		len(m.config.Masquerade), len(m.config.SNAT), len(m.config.DNAT))

	return nil
}

// Cleanup removes all applied NAT rules
func (m *Manager) Cleanup() {
	if !m.config.Enabled {
		return
	}

	log.Println("Cleaning up NAT rules...")

	// Remove rules in reverse order
	for i := len(m.appliedRules) - 1; i >= 0; i-- {
		rule := m.appliedRules[i]
		// Replace -A (add) with -D (delete)
		deleteRule := strings.Replace(rule, " -A ", " -D ", 1)
		deleteRule = strings.Replace(deleteRule, " -I ", " -D ", 1)

		args := strings.Fields(deleteRule)
		if len(args) > 0 {
			cmd := exec.Command("iptables", args...)
			if output, err := cmd.CombinedOutput(); err != nil {
				log.Printf("Warning: failed to remove rule: %s: %v", strings.TrimSpace(string(output)), err)
			}
		}
	}

	m.appliedRules = []string{}
	log.Println("NAT rules cleaned up")
}

// enableIPForwarding enables IPv4 forwarding via sysctl
func (m *Manager) enableIPForwarding() error {
	cmd := exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	log.Println("IP forwarding enabled")
	return nil
}

// applyMasquerade applies a MASQUERADE rule
func (m *Manager) applyMasquerade(rule MasqueradeRule) error {
	// iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
	ruleStr := fmt.Sprintf("-t nat -A POSTROUTING -o %s -j MASQUERADE", rule.Interface)

	// Check if rule already exists
	checkArgs := strings.Replace(ruleStr, " -A ", " -C ", 1)
	checkCmd := exec.Command("iptables", strings.Fields(checkArgs)...)
	if checkCmd.Run() == nil {
		log.Printf("Masquerade rule for %s already exists", rule.Interface)
		return nil
	}

	// Apply the rule
	cmd := exec.Command("iptables", strings.Fields(ruleStr)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}

	m.appliedRules = append(m.appliedRules, ruleStr)
	log.Printf("Applied MASQUERADE rule: -o %s", rule.Interface)
	return nil
}

// applySNAT applies a Source NAT rule
func (m *Manager) applySNAT(rule SNATRule) error {
	// iptables -t nat -A POSTROUTING -s 10.250.0.0/24 -d 192.168.250.0/24 -o wg0 -j SNAT --to-source 192.168.250.8
	ruleStr := fmt.Sprintf("-t nat -A POSTROUTING -s %s -d %s -o %s -j SNAT --to-source %s",
		rule.Source, rule.Destination, rule.Interface, rule.ToSource)

	// Check if rule already exists
	checkArgs := strings.Replace(ruleStr, " -A ", " -C ", 1)
	checkCmd := exec.Command("iptables", strings.Fields(checkArgs)...)
	if checkCmd.Run() == nil {
		log.Printf("SNAT rule already exists: %s -> %s via %s", rule.Source, rule.Destination, rule.Interface)
		return nil
	}

	// Apply the rule
	cmd := exec.Command("iptables", strings.Fields(ruleStr)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}

	m.appliedRules = append(m.appliedRules, ruleStr)
	log.Printf("Applied SNAT rule: %s -> %s via %s (source: %s)",
		rule.Source, rule.Destination, rule.Interface, rule.ToSource)
	return nil
}

// applyDNAT applies a Destination NAT (port forwarding) rule
func (m *Manager) applyDNAT(rule DNATRule) error {
	// iptables -t nat -A PREROUTING -i eth0 -p tcp --dport 8080 -j DNAT --to-destination 10.250.2.5:80
	ruleStr := fmt.Sprintf("-t nat -A PREROUTING -i %s -p %s --dport %d -j DNAT --to-destination %s",
		rule.Interface, rule.Protocol, rule.Port, rule.ToDestination)

	// Check if rule already exists
	checkArgs := strings.Replace(ruleStr, " -A ", " -C ", 1)
	checkCmd := exec.Command("iptables", strings.Fields(checkArgs)...)
	if checkCmd.Run() == nil {
		log.Printf("DNAT rule already exists: %s:%d -> %s", rule.Interface, rule.Port, rule.ToDestination)
		return nil
	}

	// Apply the rule
	cmd := exec.Command("iptables", strings.Fields(ruleStr)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}

	m.appliedRules = append(m.appliedRules, ruleStr)
	log.Printf("Applied DNAT rule: %s %s:%d -> %s",
		rule.Protocol, rule.Interface, rule.Port, rule.ToDestination)
	return nil
}
