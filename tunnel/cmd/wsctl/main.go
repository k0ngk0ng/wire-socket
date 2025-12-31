package main

import (
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"wire-socket-tunnel/internal/database"

	"gopkg.in/yaml.v3"
)

// Config represents the tunnel configuration
type Config struct {
	Database struct {
		Path string `yaml:"path"`
	} `yaml:"database"`
	WireGuard struct {
		DeviceName string `yaml:"device_name"`
	} `yaml:"wireguard"`
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Load config
	configPath := os.Getenv("WSCTL_CONFIG")
	if configPath == "" {
		configPath = "config.yaml"
	}

	config, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	db, err := database.NewDB(config.Database.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to database: %v\n", err)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "route":
		handleRouteCommand(db, config, os.Args[2:])
	case "nat":
		handleNATCommand(db, config, os.Args[2:])
	case "peer":
		handlePeerCommand(db, os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`wsctl - WireSocket Tunnel CLI

Usage: wsctl <command> [subcommand] [options]

Commands:
  route    Manage routes
    list                      List all routes
    create <cidr> [options]   Create a new route
    update <id> [options]     Update a route
    delete <id>               Delete a route
    apply                     Apply routes to system

  nat      Manage NAT rules
    list                      List all NAT rules
    create <type> [options]   Create a NAT rule (masquerade|snat|dnat|tcpmss)
    update <id> [options]     Update a NAT rule
    delete <id>               Delete a NAT rule
    apply                     Apply NAT rules to system

  peer     Manage WireGuard peers
    list                      List allocated IPs/peers

Environment:
  WSCTL_CONFIG    Path to config file (default: config.yaml)`)
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	if config.Database.Path == "" {
		config.Database.Path = "tunnel.db"
	}

	return &config, nil
}

// ============ Route Commands ============

func handleRouteCommand(db *database.DB, config *Config, args []string) {
	if len(args) == 0 {
		args = []string{"list"}
	}

	switch args[0] {
	case "list", "ls":
		listRoutes(db)
	case "create", "add":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl route create <cidr> [options]")
			os.Exit(1)
		}
		createRoute(db, args[1], args[2:])
	case "update", "edit":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl route update <id> [options]")
			os.Exit(1)
		}
		updateRoute(db, args[1], args[2:])
	case "delete", "rm":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl route delete <id>")
			os.Exit(1)
		}
		deleteRoute(db, args[1])
	case "apply":
		applyRoutes(db, config)
	default:
		fmt.Fprintf(os.Stderr, "Unknown route subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

func listRoutes(db *database.DB) {
	var routes []database.Route
	if err := db.Find(&routes).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(routes) == 0 {
		fmt.Println("No routes configured")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tCIDR\tGATEWAY\tDEVICE\tMETRIC\tPUSH\tSERVER\tCOMMENT\tENABLED")
	for _, r := range routes {
		gateway := r.Gateway
		if gateway == "" {
			gateway = "-"
		}
		device := r.Device
		if device == "" {
			device = "-"
		}
		comment := r.Comment
		if comment == "" {
			comment = "-"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%d\t%v\t%v\t%s\t%v\n",
			r.ID, r.CIDR, gateway, device, r.Metric,
			r.PushToClient, r.ApplyOnServer, comment, r.Enabled)
	}
	w.Flush()
}

func createRoute(db *database.DB, cidr string, opts []string) {
	route := database.Route{
		CIDR:         cidr,
		Enabled:      true,
		PushToClient: true,
	}

	for _, opt := range opts {
		parseRouteOption(&route, opt)
	}

	if err := db.Create(&route).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Route created: ID=%d CIDR=%s\n", route.ID, route.CIDR)
}

func updateRoute(db *database.DB, idStr string, opts []string) {
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Invalid route ID")
		os.Exit(1)
	}

	var route database.Route
	if err := db.First(&route, id).Error; err != nil {
		fmt.Fprintln(os.Stderr, "Route not found")
		os.Exit(1)
	}

	for _, opt := range opts {
		parseRouteOption(&route, opt)
	}

	if err := db.Save(&route).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Route updated: ID=%d\n", route.ID)
}

func deleteRoute(db *database.DB, idStr string) {
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Invalid route ID")
		os.Exit(1)
	}

	if err := db.Delete(&database.Route{}, id).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Route deleted: ID=%d\n", id)
}

func parseRouteOption(route *database.Route, opt string) {
	switch {
	case hasPrefix(opt, "--gateway="):
		route.Gateway = trimPrefix(opt, "--gateway=")
	case hasPrefix(opt, "--device="):
		route.Device = trimPrefix(opt, "--device=")
	case hasPrefix(opt, "--metric="):
		if v, err := strconv.Atoi(trimPrefix(opt, "--metric=")); err == nil {
			route.Metric = v
		}
	case hasPrefix(opt, "--comment="):
		route.Comment = trimPrefix(opt, "--comment=")
	case hasPrefix(opt, "--enabled="):
		route.Enabled = trimPrefix(opt, "--enabled=") == "true"
	case hasPrefix(opt, "--push="):
		route.PushToClient = trimPrefix(opt, "--push=") == "true"
	case hasPrefix(opt, "--apply="):
		route.ApplyOnServer = trimPrefix(opt, "--apply=") == "true"
	}
}

func applyRoutes(db *database.DB, config *Config) {
	var routes []database.Route
	if err := db.Where("enabled = ? AND apply_on_server = ?", true, true).Find(&routes).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(routes) == 0 {
		fmt.Println("No routes to apply")
		return
	}

	fmt.Printf("Would apply %d routes (implementation pending)\n", len(routes))
	for _, r := range routes {
		fmt.Printf("  - %s via %s dev %s metric %d\n", r.CIDR, r.Gateway, r.Device, r.Metric)
	}
}

// ============ NAT Commands ============

func handleNATCommand(db *database.DB, config *Config, args []string) {
	if len(args) == 0 {
		args = []string{"list"}
	}

	switch args[0] {
	case "list", "ls":
		listNATRules(db)
	case "create", "add":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl nat create <type> [options]")
			os.Exit(1)
		}
		createNATRule(db, args[1], args[2:])
	case "delete", "rm":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl nat delete <id>")
			os.Exit(1)
		}
		deleteNATRule(db, args[1])
	case "apply":
		fmt.Println("NAT apply: implementation pending")
	default:
		fmt.Fprintf(os.Stderr, "Unknown nat subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

func listNATRules(db *database.DB) {
	var rules []database.NATRule
	if err := db.Find(&rules).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(rules) == 0 {
		fmt.Println("No NAT rules configured")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTYPE\tINTERFACE\tDETAILS\tENABLED")
	for _, r := range rules {
		details := ""
		switch r.Type {
		case database.NATTypeMasquerade:
			details = "-"
		case database.NATTypeSNAT:
			details = fmt.Sprintf("%s -> %s", r.Source, r.ToSource)
		case database.NATTypeDNAT:
			details = fmt.Sprintf("%s:%d -> %s", r.Protocol, r.Port, r.ToDestination)
		case database.NATTypeTCPMSS:
			details = fmt.Sprintf("MSS=%d", r.MSS)
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%v\n", r.ID, r.Type, r.Interface, details, r.Enabled)
	}
	w.Flush()
}

func createNATRule(db *database.DB, natType string, opts []string) {
	rule := database.NATRule{
		Type:    database.NATType(natType),
		Enabled: true,
	}

	for _, opt := range opts {
		switch {
		case hasPrefix(opt, "--interface="):
			rule.Interface = trimPrefix(opt, "--interface=")
		case hasPrefix(opt, "--source="):
			rule.Source = trimPrefix(opt, "--source=")
		case hasPrefix(opt, "--dest="):
			rule.Destination = trimPrefix(opt, "--dest=")
		case hasPrefix(opt, "--to-source="):
			rule.ToSource = trimPrefix(opt, "--to-source=")
		case hasPrefix(opt, "--to-dest="):
			rule.ToDestination = trimPrefix(opt, "--to-dest=")
		case hasPrefix(opt, "--protocol="):
			rule.Protocol = trimPrefix(opt, "--protocol=")
		case hasPrefix(opt, "--port="):
			if v, err := strconv.Atoi(trimPrefix(opt, "--port=")); err == nil {
				rule.Port = v
			}
		case hasPrefix(opt, "--mss="):
			if v, err := strconv.Atoi(trimPrefix(opt, "--mss=")); err == nil {
				rule.MSS = v
			}
		}
	}

	if err := db.Create(&rule).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("NAT rule created: ID=%d Type=%s\n", rule.ID, rule.Type)
}

func deleteNATRule(db *database.DB, idStr string) {
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Invalid rule ID")
		os.Exit(1)
	}

	if err := db.Delete(&database.NATRule{}, id).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("NAT rule deleted: ID=%d\n", id)
}

// ============ Peer Commands ============

func handlePeerCommand(db *database.DB, args []string) {
	if len(args) == 0 {
		args = []string{"list"}
	}

	switch args[0] {
	case "list", "ls":
		listPeers(db)
	default:
		fmt.Fprintf(os.Stderr, "Unknown peer subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

func listPeers(db *database.DB) {
	var peers []database.AllocatedIP
	if err := db.Find(&peers).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(peers) == 0 {
		fmt.Println("No peers allocated")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tUSER_ID\tUSERNAME\tIP\tPUBLIC_KEY")
	for _, p := range peers {
		pubKey := p.PublicKey
		if len(pubKey) > 16 {
			pubKey = pubKey[:16] + "..."
		}
		fmt.Fprintf(w, "%d\t%d\t%s\t%s\t%s\n", p.ID, p.UserID, p.Username, p.IP, pubKey)
	}
	w.Flush()
}

// Helper functions
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func trimPrefix(s, prefix string) string {
	if hasPrefix(s, prefix) {
		return s[len(prefix):]
	}
	return s
}
