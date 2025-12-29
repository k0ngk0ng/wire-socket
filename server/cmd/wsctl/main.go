package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"wire-socket-server/internal/database"
	"wire-socket-server/internal/nat"
	"wire-socket-server/internal/route"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

// Config represents the server configuration (minimal for wsctl)
type Config struct {
	Database struct {
		Path string `yaml:"path"`
	} `yaml:"database"`
	WireGuard struct {
		DeviceName string `yaml:"device_name"`
	} `yaml:"wireguard"`
	NAT struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"nat"`
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Load config
	configPath := "config.yaml"
	if envPath := os.Getenv("WSCTL_CONFIG"); envPath != "" {
		configPath = envPath
	}

	config, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Initialize database
	db, err := database.NewDB(config.Database.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to database: %v\n", err)
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	// User commands
	case "user", "users":
		handleUserCommand(db, args)
	// Route commands
	case "route", "routes":
		handleRouteCommand(db, config, args)
	// NAT commands
	case "nat":
		handleNATCommand(db, config, args)
	// Group commands
	case "group", "groups":
		handleGroupCommand(db, args)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

func printUsage() {
	fmt.Println(`wsctl - WireSocket Server Control Tool

Usage: wsctl <command> [subcommand] [options]

Commands:
  user list [--sort=<field>]    List all users
    --sort=id|username|email|created_at  Sort by field (prefix with - for desc)
  user get <id>                 Get user details
  user create <username> <email> <password> [--admin]
                                Create a new user
  user update <id> [options]    Update user
    --username=<name>           Set username
    --email=<email>             Set email
    --password=<pwd>            Set password
    --active=true|false         Set active status
    --admin=true|false          Set admin status
  user delete <id>              Delete a user

  route list [--sort=<field>]   List all routes
    --sort=id|cidr|enabled|created_at    Sort by field (prefix with - for desc)
  route create <cidr> [options] Create a new route
    --gateway=<ip>              Next hop gateway (for server-side routing)
    --device=<dev>              Interface (defaults to wg device)
    --metric=<num>              Route priority (lower = higher priority)
    --comment=<text>            Comment
    --push-to-client=true|false Push to VPN clients (default: true)
    --apply-on-server=true|false Apply on server side (default: false)
  route update <id> [options]   Update route
    --cidr=<cidr>               Set CIDR
    --gateway=<ip>              Set gateway
    --device=<dev>              Set device
    --metric=<num>              Set metric
    --comment=<text>            Set comment
    --enabled=true|false        Set enabled status
    --push-to-client=true|false Push to clients
    --apply-on-server=true|false Apply on server
  route delete <id>             Delete a route
  route apply                   Apply routes to server routing table

  nat list [--sort=<field>]     List all NAT rules
    --sort=id|type|interface|enabled     Sort by field (prefix with - for desc)
  nat create <type> [options]   Create NAT rule
    For masquerade:
      nat create masquerade --interface=eth0
    For snat:
      nat create snat --interface=wg0 --source=10.0.0.0/24 --dest=192.168.1.0/24 --to-source=192.168.1.1
    For dnat:
      nat create dnat --interface=eth0 --protocol=tcp --port=8080 --to-dest=10.0.0.5:80
    For tcpmss (MSS clamping to prevent MTU issues):
      nat create tcpmss --interface=wg0 --source=10.0.0.0/24 --mss=1360
  nat update <id> [options]     Update NAT rule
  nat delete <id>               Delete NAT rule
  nat apply                     Apply NAT rules to iptables

  group list [--sort=<field>]   List all groups
    --sort=id|name|created_at            Sort by field (prefix with - for desc)
  group create <name> [options] Create a new group
    --description=<text>        Group description
  group get <id>                Get group details (with users/routes)
  group update <id> [options]   Update group
    --name=<name>               Set name
    --description=<text>        Set description
  group delete <id>             Delete a group
  group add-user <group_id> <user_id>
                                Add user to group
  group remove-user <group_id> <user_id>
                                Remove user from group
  group add-route <group_id> <route_id>
                                Add route to group
  group remove-route <group_id> <route_id>
                                Remove route from group

Environment:
  WSCTL_CONFIG                  Config file path (default: config.yaml)

Examples:
  wsctl user list
  wsctl user create alice alice@example.com secret123 --admin
  wsctl route create 192.168.1.0/24 "Internal network"
  wsctl nat create masquerade --interface=eth0
  wsctl nat apply
  wsctl group create developers --description="Dev team"
  wsctl group add-user 1 2
  wsctl group add-route 1 3`)
}

// ============ User Commands ============

func handleUserCommand(db *database.DB, args []string) {
	if len(args) == 0 {
		args = []string{"list"}
	}

	switch args[0] {
	case "list", "ls":
		listUsers(db, args[1:])
	case "get":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl user get <id>")
			os.Exit(1)
		}
		getUser(db, args[1])
	case "create", "add":
		if len(args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl user create <username> <email> <password> [--admin]")
			os.Exit(1)
		}
		isAdmin := contains(args, "--admin")
		createUser(db, args[1], args[2], args[3], isAdmin)
	case "update", "edit":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl user update <id> [options]")
			os.Exit(1)
		}
		updateUser(db, args[1], args[2:])
	case "delete", "rm":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl user delete <id>")
			os.Exit(1)
		}
		deleteUser(db, args[1])
	default:
		fmt.Fprintf(os.Stderr, "Unknown user subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

func listUsers(db *database.DB, args []string) {
	sortField, desc := parseSortOption(args, []string{"id", "username", "email", "created_at"})
	orderClause := buildOrderClause(sortField, desc)

	var users []database.User
	if err := db.Order(orderClause).Find(&users).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tUSERNAME\tEMAIL\tACTIVE\tADMIN")
	for _, u := range users {
		fmt.Fprintf(w, "%d\t%s\t%s\t%v\t%v\n", u.ID, u.Username, u.Email, u.IsActive, u.IsAdmin)
	}
	w.Flush()
}

func getUser(db *database.DB, idStr string) {
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid ID: %s\n", idStr)
		os.Exit(1)
	}

	var user database.User
	if err := db.First(&user, id).Error; err != nil {
		fmt.Fprintf(os.Stderr, "User not found: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("ID:       %d\n", user.ID)
	fmt.Printf("Username: %s\n", user.Username)
	fmt.Printf("Email:    %s\n", user.Email)
	fmt.Printf("Active:   %v\n", user.IsActive)
	fmt.Printf("Admin:    %v\n", user.IsAdmin)
	fmt.Printf("Created:  %s\n", user.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Updated:  %s\n", user.UpdatedAt.Format("2006-01-02 15:04:05"))
}

func createUser(db *database.DB, username, email, password string, isAdmin bool) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error hashing password: %v\n", err)
		os.Exit(1)
	}

	user := database.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hashedPassword),
		IsActive:     true,
		IsAdmin:      isAdmin,
	}

	if err := db.Create(&user).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error creating user: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("User created: ID=%d, Username=%s\n", user.ID, user.Username)
}

func updateUser(db *database.DB, idStr string, opts []string) {
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid ID: %s\n", idStr)
		os.Exit(1)
	}

	var user database.User
	if err := db.First(&user, id).Error; err != nil {
		fmt.Fprintf(os.Stderr, "User not found: %v\n", err)
		os.Exit(1)
	}

	for _, opt := range opts {
		if strings.HasPrefix(opt, "--username=") {
			user.Username = strings.TrimPrefix(opt, "--username=")
		} else if strings.HasPrefix(opt, "--email=") {
			user.Email = strings.TrimPrefix(opt, "--email=")
		} else if strings.HasPrefix(opt, "--password=") {
			pwd := strings.TrimPrefix(opt, "--password=")
			hashedPassword, err := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.DefaultCost)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error hashing password: %v\n", err)
				os.Exit(1)
			}
			user.PasswordHash = string(hashedPassword)
		} else if strings.HasPrefix(opt, "--active=") {
			user.IsActive = strings.TrimPrefix(opt, "--active=") == "true"
		} else if strings.HasPrefix(opt, "--admin=") {
			user.IsAdmin = strings.TrimPrefix(opt, "--admin=") == "true"
		}
	}

	if err := db.Save(&user).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error updating user: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("User updated: ID=%d\n", user.ID)
}

func deleteUser(db *database.DB, idStr string) {
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid ID: %s\n", idStr)
		os.Exit(1)
	}

	var user database.User
	if err := db.First(&user, id).Error; err != nil {
		fmt.Fprintf(os.Stderr, "User not found: %v\n", err)
		os.Exit(1)
	}

	// Delete user's allocations and sessions
	db.Where("user_id = ?", user.ID).Delete(&database.AllocatedIP{})
	db.Where("user_id = ?", user.ID).Delete(&database.Session{})

	if err := db.Delete(&user).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting user: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("User deleted: ID=%d\n", user.ID)
}

// ============ Route Commands ============

func handleRouteCommand(db *database.DB, config *Config, args []string) {
	if len(args) == 0 {
		args = []string{"list"}
	}

	switch args[0] {
	case "list", "ls":
		listRoutes(db, args[1:])
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

func listRoutes(db *database.DB, args []string) {
	sortField, desc := parseSortOption(args, []string{"id", "cidr", "enabled", "created_at"})
	orderClause := buildOrderClause(sortField, desc)

	var routes []database.Route
	if err := db.Order(orderClause).Find(&routes).Error; err != nil {
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
			device = "(default)"
		}
		metric := "-"
		if r.Metric > 0 {
			metric = strconv.Itoa(r.Metric)
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%v\t%v\t%s\t%v\n", r.ID, r.CIDR, gateway, device, metric, r.PushToClient, r.ApplyOnServer, r.Comment, r.Enabled)
	}
	w.Flush()
}

func createRoute(db *database.DB, cidr string, opts []string) {
	route := database.Route{
		CIDR:          cidr,
		Enabled:       true,
		PushToClient:  true,
		ApplyOnServer: false,
	}

	for _, opt := range opts {
		if strings.HasPrefix(opt, "--gateway=") {
			route.Gateway = strings.TrimPrefix(opt, "--gateway=")
		} else if strings.HasPrefix(opt, "--device=") {
			route.Device = strings.TrimPrefix(opt, "--device=")
		} else if strings.HasPrefix(opt, "--metric=") {
			metric, _ := strconv.Atoi(strings.TrimPrefix(opt, "--metric="))
			route.Metric = metric
		} else if strings.HasPrefix(opt, "--comment=") {
			route.Comment = strings.TrimPrefix(opt, "--comment=")
		} else if strings.HasPrefix(opt, "--push-to-client=") {
			value := strings.TrimPrefix(opt, "--push-to-client=")
			route.PushToClient = value == "true"
		} else if strings.HasPrefix(opt, "--apply-on-server=") {
			value := strings.TrimPrefix(opt, "--apply-on-server=")
			route.ApplyOnServer = value == "true"
		} else if strings.HasPrefix(opt, "--enabled=") {
			value := strings.TrimPrefix(opt, "--enabled=")
			route.Enabled = value == "true"
		} else if !strings.HasPrefix(opt, "--") {
			// Legacy: treat non-option argument as comment
			route.Comment = opt
		}
	}

	if err := db.Create(&route).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error creating route: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Route created: ID=%d, CIDR=%s, PushToClient=%v, ApplyOnServer=%v\n", route.ID, route.CIDR, route.PushToClient, route.ApplyOnServer)
}

func updateRoute(db *database.DB, idStr string, opts []string) {
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid ID: %s\n", idStr)
		os.Exit(1)
	}

	var route database.Route
	if err := db.First(&route, id).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Route not found: %v\n", err)
		os.Exit(1)
	}

	for _, opt := range opts {
		if strings.HasPrefix(opt, "--cidr=") {
			route.CIDR = strings.TrimPrefix(opt, "--cidr=")
		} else if strings.HasPrefix(opt, "--gateway=") {
			route.Gateway = strings.TrimPrefix(opt, "--gateway=")
		} else if strings.HasPrefix(opt, "--device=") {
			route.Device = strings.TrimPrefix(opt, "--device=")
		} else if strings.HasPrefix(opt, "--metric=") {
			metric, _ := strconv.Atoi(strings.TrimPrefix(opt, "--metric="))
			route.Metric = metric
		} else if strings.HasPrefix(opt, "--comment=") {
			route.Comment = strings.TrimPrefix(opt, "--comment=")
		} else if strings.HasPrefix(opt, "--enabled=") {
			route.Enabled = strings.TrimPrefix(opt, "--enabled=") == "true"
		} else if strings.HasPrefix(opt, "--push-to-client=") {
			route.PushToClient = strings.TrimPrefix(opt, "--push-to-client=") == "true"
		} else if strings.HasPrefix(opt, "--apply-on-server=") {
			route.ApplyOnServer = strings.TrimPrefix(opt, "--apply-on-server=") == "true"
		}
	}

	if err := db.Save(&route).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error updating route: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Route updated: ID=%d\n", route.ID)
}

func deleteRoute(db *database.DB, idStr string) {
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid ID: %s\n", idStr)
		os.Exit(1)
	}

	var route database.Route
	if err := db.First(&route, id).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Route not found: %v\n", err)
		os.Exit(1)
	}

	if err := db.Delete(&route).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting route: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Route deleted: ID=%d\n", route.ID)
}

func applyRoutes(db *database.DB, config *Config) {
	var dbRoutes []database.Route
	if err := db.Where("enabled = ? AND apply_on_server = ?", true, true).Find(&dbRoutes).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error loading routes: %v\n", err)
		os.Exit(1)
	}

	if len(dbRoutes) == 0 {
		fmt.Println("No routes to apply (no enabled routes with apply_on_server=true)")
		return
	}

	// Build route config
	var routes []route.Route
	for _, r := range dbRoutes {
		routes = append(routes, route.Route{
			CIDR:    r.CIDR,
			Gateway: r.Gateway,
			Device:  r.Device,
			Metric:  r.Metric,
		})
	}

	routeConfig := route.Config{
		DefaultDevice: config.WireGuard.DeviceName,
		Routes:        routes,
	}

	// Apply routes
	manager := route.NewManager(routeConfig)
	if err := manager.Apply(); err != nil {
		fmt.Fprintf(os.Stderr, "Error applying routes: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Routes applied: %d routes\n", len(routes))
}

// ============ NAT Commands ============

func handleNATCommand(db *database.DB, config *Config, args []string) {
	if len(args) == 0 {
		args = []string{"list"}
	}

	switch args[0] {
	case "list", "ls":
		listNATRules(db, args[1:])
	case "create", "add":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl nat create <type> [options]")
			os.Exit(1)
		}
		createNATRule(db, args[1], args[2:])
	case "update", "edit":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl nat update <id> [options]")
			os.Exit(1)
		}
		updateNATRule(db, args[1], args[2:])
	case "delete", "rm":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl nat delete <id>")
			os.Exit(1)
		}
		deleteNATRule(db, args[1])
	case "apply":
		applyNATRules(db, config)
	default:
		fmt.Fprintf(os.Stderr, "Unknown nat subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

func listNATRules(db *database.DB, args []string) {
	sortField, desc := parseSortOption(args, []string{"id", "type", "interface", "enabled"})
	orderClause := buildOrderClause(sortField, desc)

	var rules []database.NATRule
	if err := db.Order(orderClause).Find(&rules).Error; err != nil {
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
			details = fmt.Sprintf("%s mss=%d", r.Source, r.MSS)
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%v\n", r.ID, r.Type, r.Interface, details, r.Enabled)
	}
	w.Flush()
}

func createNATRule(db *database.DB, ruleType string, opts []string) {
	rule := database.NATRule{
		Type:    database.NATRuleType(ruleType),
		Enabled: true,
	}

	for _, opt := range opts {
		if strings.HasPrefix(opt, "--interface=") {
			rule.Interface = strings.TrimPrefix(opt, "--interface=")
		} else if strings.HasPrefix(opt, "--source=") {
			rule.Source = strings.TrimPrefix(opt, "--source=")
		} else if strings.HasPrefix(opt, "--dest=") || strings.HasPrefix(opt, "--destination=") {
			rule.Destination = strings.TrimPrefix(strings.TrimPrefix(opt, "--dest="), "--destination=")
		} else if strings.HasPrefix(opt, "--to-source=") {
			rule.ToSource = strings.TrimPrefix(opt, "--to-source=")
		} else if strings.HasPrefix(opt, "--protocol=") {
			rule.Protocol = strings.TrimPrefix(opt, "--protocol=")
		} else if strings.HasPrefix(opt, "--port=") {
			port, _ := strconv.Atoi(strings.TrimPrefix(opt, "--port="))
			rule.Port = port
		} else if strings.HasPrefix(opt, "--to-dest=") || strings.HasPrefix(opt, "--to-destination=") {
			rule.ToDestination = strings.TrimPrefix(strings.TrimPrefix(opt, "--to-dest="), "--to-destination=")
		} else if strings.HasPrefix(opt, "--comment=") {
			rule.Comment = strings.TrimPrefix(opt, "--comment=")
		} else if strings.HasPrefix(opt, "--mss=") {
			mss, _ := strconv.Atoi(strings.TrimPrefix(opt, "--mss="))
			rule.MSS = mss
		}
	}

	// Validate
	if rule.Interface == "" {
		fmt.Fprintln(os.Stderr, "Error: --interface is required")
		os.Exit(1)
	}

	switch rule.Type {
	case database.NATTypeMasquerade:
		// Interface only required
	case database.NATTypeSNAT:
		if rule.Source == "" || rule.Destination == "" || rule.ToSource == "" {
			fmt.Fprintln(os.Stderr, "Error: SNAT requires --source, --dest, and --to-source")
			os.Exit(1)
		}
	case database.NATTypeDNAT:
		if rule.Protocol == "" || rule.Port == 0 || rule.ToDestination == "" {
			fmt.Fprintln(os.Stderr, "Error: DNAT requires --protocol, --port, and --to-dest")
			os.Exit(1)
		}
	case database.NATTypeTCPMSS:
		if rule.Source == "" || rule.MSS == 0 {
			fmt.Fprintln(os.Stderr, "Error: TCPMSS requires --source and --mss")
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Error: Invalid NAT type: %s (use: masquerade, snat, dnat, tcpmss)\n", ruleType)
		os.Exit(1)
	}

	if err := db.Create(&rule).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error creating NAT rule: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("NAT rule created: ID=%d, Type=%s\n", rule.ID, rule.Type)
}

func updateNATRule(db *database.DB, idStr string, opts []string) {
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid ID: %s\n", idStr)
		os.Exit(1)
	}

	var rule database.NATRule
	if err := db.First(&rule, id).Error; err != nil {
		fmt.Fprintf(os.Stderr, "NAT rule not found: %v\n", err)
		os.Exit(1)
	}

	for _, opt := range opts {
		if strings.HasPrefix(opt, "--interface=") {
			rule.Interface = strings.TrimPrefix(opt, "--interface=")
		} else if strings.HasPrefix(opt, "--source=") {
			rule.Source = strings.TrimPrefix(opt, "--source=")
		} else if strings.HasPrefix(opt, "--dest=") || strings.HasPrefix(opt, "--destination=") {
			rule.Destination = strings.TrimPrefix(strings.TrimPrefix(opt, "--dest="), "--destination=")
		} else if strings.HasPrefix(opt, "--to-source=") {
			rule.ToSource = strings.TrimPrefix(opt, "--to-source=")
		} else if strings.HasPrefix(opt, "--protocol=") {
			rule.Protocol = strings.TrimPrefix(opt, "--protocol=")
		} else if strings.HasPrefix(opt, "--port=") {
			port, _ := strconv.Atoi(strings.TrimPrefix(opt, "--port="))
			rule.Port = port
		} else if strings.HasPrefix(opt, "--to-dest=") || strings.HasPrefix(opt, "--to-destination=") {
			rule.ToDestination = strings.TrimPrefix(strings.TrimPrefix(opt, "--to-dest="), "--to-destination=")
		} else if strings.HasPrefix(opt, "--comment=") {
			rule.Comment = strings.TrimPrefix(opt, "--comment=")
		} else if strings.HasPrefix(opt, "--enabled=") {
			rule.Enabled = strings.TrimPrefix(opt, "--enabled=") == "true"
		} else if strings.HasPrefix(opt, "--mss=") {
			mss, _ := strconv.Atoi(strings.TrimPrefix(opt, "--mss="))
			rule.MSS = mss
		}
	}

	if err := db.Save(&rule).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error updating NAT rule: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("NAT rule updated: ID=%d\n", rule.ID)
}

func deleteNATRule(db *database.DB, idStr string) {
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid ID: %s\n", idStr)
		os.Exit(1)
	}

	var rule database.NATRule
	if err := db.First(&rule, id).Error; err != nil {
		fmt.Fprintf(os.Stderr, "NAT rule not found: %v\n", err)
		os.Exit(1)
	}

	if err := db.Delete(&rule).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting NAT rule: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("NAT rule deleted: ID=%d\n", rule.ID)
}

func applyNATRules(db *database.DB, config *Config) {
	if !config.NAT.Enabled {
		fmt.Println("NAT is disabled in config.yaml")
		return
	}

	var rules []database.NATRule
	if err := db.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error loading NAT rules: %v\n", err)
		os.Exit(1)
	}

	// Build NAT config
	natConfig := nat.Config{
		Enabled: true,
	}

	for _, rule := range rules {
		switch rule.Type {
		case database.NATTypeMasquerade:
			natConfig.Masquerade = append(natConfig.Masquerade, nat.MasqueradeRule{
				Interface: rule.Interface,
			})
		case database.NATTypeSNAT:
			natConfig.SNAT = append(natConfig.SNAT, nat.SNATRule{
				Source:      rule.Source,
				Destination: rule.Destination,
				Interface:   rule.Interface,
				ToSource:    rule.ToSource,
			})
		case database.NATTypeDNAT:
			natConfig.DNAT = append(natConfig.DNAT, nat.DNATRule{
				Interface:     rule.Interface,
				Protocol:      rule.Protocol,
				Port:          rule.Port,
				ToDestination: rule.ToDestination,
			})
		case database.NATTypeTCPMSS:
			natConfig.TCPMSS = append(natConfig.TCPMSS, nat.TCPMSSRule{
				Interface: rule.Interface,
				Source:    rule.Source,
				MSS:       rule.MSS,
			})
		}
	}

	// Apply rules
	manager := nat.NewManager(natConfig)
	if err := manager.Apply(); err != nil {
		fmt.Fprintf(os.Stderr, "Error applying NAT rules: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("NAT rules applied: %d masquerade, %d SNAT, %d DNAT, %d TCPMSS\n",
		len(natConfig.Masquerade), len(natConfig.SNAT), len(natConfig.DNAT), len(natConfig.TCPMSS))
}

// Helper functions

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// parseSortOption extracts sort field from args and returns (field, isDesc, validFields)
// Returns empty string if no sort option found
func parseSortOption(args []string, validFields []string) (string, bool) {
	for _, arg := range args {
		if strings.HasPrefix(arg, "--sort=") {
			field := strings.TrimPrefix(arg, "--sort=")
			desc := false
			if strings.HasPrefix(field, "-") {
				desc = true
				field = strings.TrimPrefix(field, "-")
			}
			// Validate field
			for _, valid := range validFields {
				if field == valid {
					return field, desc
				}
			}
			fmt.Fprintf(os.Stderr, "Invalid sort field: %s (valid: %s)\n", field, strings.Join(validFields, ", "))
			os.Exit(1)
		}
	}
	return "id", false // default sort by id ascending
}

// buildOrderClause creates GORM order clause
func buildOrderClause(field string, desc bool) string {
	if desc {
		return field + " DESC"
	}
	return field + " ASC"
}

// ============ Group Commands ============

func handleGroupCommand(db *database.DB, args []string) {
	if len(args) == 0 {
		args = []string{"list"}
	}

	switch args[0] {
	case "list", "ls":
		listGroups(db, args[1:])
	case "get":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl group get <id>")
			os.Exit(1)
		}
		getGroup(db, args[1])
	case "create", "add":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl group create <name> [--description=<text>]")
			os.Exit(1)
		}
		createGroup(db, args[1], args[2:])
	case "update", "edit":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl group update <id> [options]")
			os.Exit(1)
		}
		updateGroup(db, args[1], args[2:])
	case "delete", "rm":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl group delete <id>")
			os.Exit(1)
		}
		deleteGroup(db, args[1])
	case "add-user":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl group add-user <group_id> <user_id>")
			os.Exit(1)
		}
		addUserToGroup(db, args[1], args[2])
	case "remove-user":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl group remove-user <group_id> <user_id>")
			os.Exit(1)
		}
		removeUserFromGroup(db, args[1], args[2])
	case "add-route":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl group add-route <group_id> <route_id>")
			os.Exit(1)
		}
		addRouteToGroup(db, args[1], args[2])
	case "remove-route":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl group remove-route <group_id> <route_id>")
			os.Exit(1)
		}
		removeRouteFromGroup(db, args[1], args[2])
	default:
		fmt.Fprintf(os.Stderr, "Unknown group subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

func listGroups(db *database.DB, args []string) {
	sortField, desc := parseSortOption(args, []string{"id", "name", "created_at"})
	orderClause := buildOrderClause(sortField, desc)

	var groups []database.Group
	if err := db.Order(orderClause).Find(&groups).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(groups) == 0 {
		fmt.Println("No groups configured")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tDESCRIPTION\tUSERS\tROUTES")
	for _, g := range groups {
		// Count users in group
		var userCount int64
		db.Model(&database.UserGroup{}).Where("group_id = ?", g.ID).Count(&userCount)
		// Count routes in group
		var routeCount int64
		db.Model(&database.RouteGroup{}).Where("group_id = ?", g.ID).Count(&routeCount)

		fmt.Fprintf(w, "%d\t%s\t%s\t%d\t%d\n", g.ID, g.Name, g.Description, userCount, routeCount)
	}
	w.Flush()
}

func getGroup(db *database.DB, idStr string) {
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid ID: %s\n", idStr)
		os.Exit(1)
	}

	var group database.Group
	if err := db.First(&group, id).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Group not found: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("ID:          %d\n", group.ID)
	fmt.Printf("Name:        %s\n", group.Name)
	fmt.Printf("Description: %s\n", group.Description)
	fmt.Printf("Created:     %s\n", group.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Updated:     %s\n", group.UpdatedAt.Format("2006-01-02 15:04:05"))

	// List users in group
	var userGroups []database.UserGroup
	db.Where("group_id = ?", group.ID).Preload("User").Find(&userGroups)
	if len(userGroups) > 0 {
		fmt.Printf("\nUsers (%d):\n", len(userGroups))
		for _, ug := range userGroups {
			fmt.Printf("  - [%d] %s (%s)\n", ug.User.ID, ug.User.Username, ug.User.Email)
		}
	} else {
		fmt.Printf("\nUsers: (none)\n")
	}

	// List routes in group
	var routeGroups []database.RouteGroup
	db.Where("group_id = ?", group.ID).Preload("Route").Find(&routeGroups)
	if len(routeGroups) > 0 {
		fmt.Printf("\nRoutes (%d):\n", len(routeGroups))
		for _, rg := range routeGroups {
			comment := rg.Route.Comment
			if comment == "" {
				comment = "-"
			}
			fmt.Printf("  - [%d] %s (%s)\n", rg.Route.ID, rg.Route.CIDR, comment)
		}
	} else {
		fmt.Printf("\nRoutes: (none)\n")
	}
}

func createGroup(db *database.DB, name string, opts []string) {
	group := database.Group{
		Name: name,
	}

	for _, opt := range opts {
		if strings.HasPrefix(opt, "--description=") {
			group.Description = strings.TrimPrefix(opt, "--description=")
		}
	}

	if err := db.Create(&group).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error creating group: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Group created: ID=%d, Name=%s\n", group.ID, group.Name)
}

func updateGroup(db *database.DB, idStr string, opts []string) {
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid ID: %s\n", idStr)
		os.Exit(1)
	}

	var group database.Group
	if err := db.First(&group, id).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Group not found: %v\n", err)
		os.Exit(1)
	}

	for _, opt := range opts {
		if strings.HasPrefix(opt, "--name=") {
			group.Name = strings.TrimPrefix(opt, "--name=")
		} else if strings.HasPrefix(opt, "--description=") {
			group.Description = strings.TrimPrefix(opt, "--description=")
		}
	}

	if err := db.Save(&group).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error updating group: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Group updated: ID=%d\n", group.ID)
}

func deleteGroup(db *database.DB, idStr string) {
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid ID: %s\n", idStr)
		os.Exit(1)
	}

	var group database.Group
	if err := db.First(&group, id).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Group not found: %v\n", err)
		os.Exit(1)
	}

	// Delete group memberships
	db.Where("group_id = ?", group.ID).Delete(&database.UserGroup{})
	db.Where("group_id = ?", group.ID).Delete(&database.RouteGroup{})

	if err := db.Delete(&group).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting group: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Group deleted: ID=%d\n", group.ID)
}

func addUserToGroup(db *database.DB, groupIDStr, userIDStr string) {
	groupID, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid group ID: %s\n", groupIDStr)
		os.Exit(1)
	}
	userID, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid user ID: %s\n", userIDStr)
		os.Exit(1)
	}

	// Verify group exists
	var group database.Group
	if err := db.First(&group, groupID).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Group not found: %v\n", err)
		os.Exit(1)
	}

	// Verify user exists
	var user database.User
	if err := db.First(&user, userID).Error; err != nil {
		fmt.Fprintf(os.Stderr, "User not found: %v\n", err)
		os.Exit(1)
	}

	userGroup := database.UserGroup{
		UserID:  uint(userID),
		GroupID: uint(groupID),
	}

	if err := db.Create(&userGroup).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error adding user to group: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("User %s added to group %s\n", user.Username, group.Name)
}

func removeUserFromGroup(db *database.DB, groupIDStr, userIDStr string) {
	groupID, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid group ID: %s\n", groupIDStr)
		os.Exit(1)
	}
	userID, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid user ID: %s\n", userIDStr)
		os.Exit(1)
	}

	result := db.Where("group_id = ? AND user_id = ?", groupID, userID).Delete(&database.UserGroup{})
	if result.Error != nil {
		fmt.Fprintf(os.Stderr, "Error removing user from group: %v\n", result.Error)
		os.Exit(1)
	}

	if result.RowsAffected == 0 {
		fmt.Fprintln(os.Stderr, "User is not in this group")
		os.Exit(1)
	}

	fmt.Printf("User removed from group\n")
}

func addRouteToGroup(db *database.DB, groupIDStr, routeIDStr string) {
	groupID, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid group ID: %s\n", groupIDStr)
		os.Exit(1)
	}
	routeID, err := strconv.ParseUint(routeIDStr, 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid route ID: %s\n", routeIDStr)
		os.Exit(1)
	}

	// Verify group exists
	var group database.Group
	if err := db.First(&group, groupID).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Group not found: %v\n", err)
		os.Exit(1)
	}

	// Verify route exists
	var route database.Route
	if err := db.First(&route, routeID).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Route not found: %v\n", err)
		os.Exit(1)
	}

	routeGroup := database.RouteGroup{
		RouteID: uint(routeID),
		GroupID: uint(groupID),
	}

	if err := db.Create(&routeGroup).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error adding route to group: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Route %s added to group %s\n", route.CIDR, group.Name)
}

func removeRouteFromGroup(db *database.DB, groupIDStr, routeIDStr string) {
	groupID, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid group ID: %s\n", groupIDStr)
		os.Exit(1)
	}
	routeID, err := strconv.ParseUint(routeIDStr, 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid route ID: %s\n", routeIDStr)
		os.Exit(1)
	}

	result := db.Where("group_id = ? AND route_id = ?", groupID, routeID).Delete(&database.RouteGroup{})
	if result.Error != nil {
		fmt.Fprintf(os.Stderr, "Error removing route from group: %v\n", result.Error)
		os.Exit(1)
	}

	if result.RowsAffected == 0 {
		fmt.Fprintln(os.Stderr, "Route is not in this group")
		os.Exit(1)
	}

	fmt.Printf("Route removed from group\n")
}
