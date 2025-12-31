package main

import (
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"wire-socket-auth/internal/database"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

// Config represents the auth configuration
type Config struct {
	Database struct {
		Path string `yaml:"path"`
	} `yaml:"database"`
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
	case "user":
		handleUserCommand(db, os.Args[2:])
	case "tunnel":
		handleTunnelCommand(db, os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`wsctl - WireSocket Auth CLI

Usage: wsctl <command> [subcommand] [options]

Commands:
  user     Manage users
    list                          List all users
    get <id>                      Get user details
    create <username> <email> <password> [--admin]   Create a user
    update <id> [options]         Update a user
    delete <id>                   Delete a user

  tunnel   Manage tunnel nodes
    list                          List all tunnel nodes
    get <id>                      Get tunnel details
    delete <id>                   Delete a tunnel

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
		config.Database.Path = "auth.db"
	}

	return &config, nil
}

// ============ User Commands ============

func handleUserCommand(db *database.DB, args []string) {
	if len(args) == 0 {
		args = []string{"list"}
	}

	switch args[0] {
	case "list", "ls":
		listUsers(db)
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
		isAdmin := len(args) > 4 && args[4] == "--admin"
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

func listUsers(db *database.DB) {
	var users []database.User
	if err := db.Find(&users).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(users) == 0 {
		fmt.Println("No users found")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tUSERNAME\tEMAIL\tACTIVE\tADMIN\tCREATED")
	for _, u := range users {
		fmt.Fprintf(w, "%d\t%s\t%s\t%v\t%v\t%s\n",
			u.ID, u.Username, u.Email, u.IsActive, u.IsAdmin,
			u.CreatedAt.Format("2006-01-02"))
	}
	w.Flush()
}

func getUser(db *database.DB, idStr string) {
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Invalid user ID")
		os.Exit(1)
	}

	var user database.User
	if err := db.First(&user, id).Error; err != nil {
		fmt.Fprintln(os.Stderr, "User not found")
		os.Exit(1)
	}

	fmt.Printf("ID:       %d\n", user.ID)
	fmt.Printf("Username: %s\n", user.Username)
	fmt.Printf("Email:    %s\n", user.Email)
	fmt.Printf("Active:   %v\n", user.IsActive)
	fmt.Printf("Admin:    %v\n", user.IsAdmin)
	fmt.Printf("Created:  %s\n", user.CreatedAt.Format("2006-01-02 15:04:05"))

	// Get tunnel access
	tunnels, _ := db.GetUserAllowedTunnels(user.ID)
	if len(tunnels) > 0 {
		fmt.Printf("Tunnels:  %v\n", tunnels)
	}
}

func createUser(db *database.DB, username, email, password string, isAdmin bool) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error hashing password: %v\n", err)
		os.Exit(1)
	}

	user := database.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(passwordHash),
		IsActive:     true,
		IsAdmin:      isAdmin,
	}

	if err := db.Create(&user).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("User created: ID=%d Username=%s\n", user.ID, user.Username)
}

func updateUser(db *database.DB, idStr string, opts []string) {
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Invalid user ID")
		os.Exit(1)
	}

	var user database.User
	if err := db.First(&user, id).Error; err != nil {
		fmt.Fprintln(os.Stderr, "User not found")
		os.Exit(1)
	}

	for _, opt := range opts {
		switch {
		case hasPrefix(opt, "--username="):
			user.Username = trimPrefix(opt, "--username=")
		case hasPrefix(opt, "--email="):
			user.Email = trimPrefix(opt, "--email=")
		case hasPrefix(opt, "--password="):
			passwordHash, _ := bcrypt.GenerateFromPassword([]byte(trimPrefix(opt, "--password=")), bcrypt.DefaultCost)
			user.PasswordHash = string(passwordHash)
		case hasPrefix(opt, "--active="):
			user.IsActive = trimPrefix(opt, "--active=") == "true"
		case hasPrefix(opt, "--admin="):
			user.IsAdmin = trimPrefix(opt, "--admin=") == "true"
		}
	}

	if err := db.Save(&user).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("User updated: ID=%d\n", user.ID)
}

func deleteUser(db *database.DB, idStr string) {
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Invalid user ID")
		os.Exit(1)
	}

	if err := db.Delete(&database.User{}, id).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("User deleted: ID=%d\n", id)
}

// ============ Tunnel Commands ============

func handleTunnelCommand(db *database.DB, args []string) {
	if len(args) == 0 {
		args = []string{"list"}
	}

	switch args[0] {
	case "list", "ls":
		listTunnels(db)
	case "get":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl tunnel get <id>")
			os.Exit(1)
		}
		getTunnel(db, args[1])
	case "delete", "rm":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: wsctl tunnel delete <id>")
			os.Exit(1)
		}
		deleteTunnel(db, args[1])
	default:
		fmt.Fprintf(os.Stderr, "Unknown tunnel subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

func listTunnels(db *database.DB) {
	var tunnels []database.Tunnel
	if err := db.Find(&tunnels).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(tunnels) == 0 {
		fmt.Println("No tunnels registered")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tREGION\tURL\tACTIVE\tLAST_SEEN")
	for _, t := range tunnels {
		lastSeen := "-"
		if !t.LastSeen.IsZero() {
			lastSeen = t.LastSeen.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%v\t%s\n",
			t.ID, t.Name, t.Region, t.URL, t.IsActive, lastSeen)
	}
	w.Flush()
}

func getTunnel(db *database.DB, id string) {
	var tunnel database.Tunnel
	if err := db.First(&tunnel, "id = ?", id).Error; err != nil {
		fmt.Fprintln(os.Stderr, "Tunnel not found")
		os.Exit(1)
	}

	fmt.Printf("ID:          %s\n", tunnel.ID)
	fmt.Printf("Name:        %s\n", tunnel.Name)
	fmt.Printf("Region:      %s\n", tunnel.Region)
	fmt.Printf("URL:         %s\n", tunnel.URL)
	fmt.Printf("Internal:    %s\n", tunnel.InternalURL)
	fmt.Printf("Active:      %v\n", tunnel.IsActive)
	fmt.Printf("Last Seen:   %s\n", tunnel.LastSeen.Format("2006-01-02 15:04:05"))
	fmt.Printf("Created:     %s\n", tunnel.CreatedAt.Format("2006-01-02 15:04:05"))
}

func deleteTunnel(db *database.DB, id string) {
	// Delete access records first
	db.Where("tunnel_id = ?", id).Delete(&database.UserTunnelAccess{})

	if err := db.Delete(&database.Tunnel{}, "id = ?", id).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Tunnel deleted: ID=%s\n", id)
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
