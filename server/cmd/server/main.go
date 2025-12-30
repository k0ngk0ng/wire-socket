package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"wire-socket-server/internal/admin"
	"wire-socket-server/internal/api"
	"wire-socket-server/internal/auth"
	"wire-socket-server/internal/database"
	"wire-socket-server/internal/nat"
	"wire-socket-server/internal/tunnel"
	"wire-socket-server/internal/wireguard"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"gopkg.in/yaml.v3"
)

// Version is set at build time via -ldflags
var Version = "dev"

// Log level constants
const (
	LogLevelDebug = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

var currentLogLevel = LogLevelInfo

// setupLogging configures log level based on config
func setupLogging(level string) {
	switch strings.ToLower(level) {
	case "debug":
		currentLogLevel = LogLevelDebug
		gin.SetMode(gin.DebugMode)
	case "warn", "warning":
		currentLogLevel = LogLevelWarn
		gin.SetMode(gin.ReleaseMode)
	case "error":
		currentLogLevel = LogLevelError
		gin.SetMode(gin.ReleaseMode)
		log.SetOutput(io.Discard) // Suppress all but fatal logs
	default: // "info" or unset
		currentLogLevel = LogLevelInfo
		gin.SetMode(gin.ReleaseMode)
	}
}

// Config represents the server configuration
type Config struct {
	Server struct {
		Address  string `yaml:"address"`
		LogLevel string `yaml:"log_level"` // debug, info, warn, error (default: info)
		TLS      *struct {
			CertFile string `yaml:"cert_file"`
			KeyFile  string `yaml:"key_file"`
		} `yaml:"tls"`
	} `yaml:"server"`
	Database struct {
		Path   string `yaml:"path"`
		Driver string `yaml:"driver"`
		DSN    string `yaml:"dsn"`
	} `yaml:"database"`
	WireGuard struct {
		DeviceName string   `yaml:"device_name"`
		ListenPort int      `yaml:"listen_port"`
		Subnet     string   `yaml:"subnet"`
		DNS        string   `yaml:"dns"`
		Endpoint   string   `yaml:"endpoint"`
		PrivateKey string   `yaml:"private_key"`
		PublicKey  string   `yaml:"public_key"`
		Mode       string   `yaml:"mode"`   // "kernel" or "userspace"
		Routes     []string `yaml:"routes"` // Routes to push to clients
	} `yaml:"wireguard"`
	Auth struct {
		JWTSecret           string `yaml:"jwt_secret"`
		AllowRegistration   bool   `yaml:"allow_registration"` // Default: false (disabled)
	} `yaml:"auth"`
	Tunnel struct {
		Enabled    bool   `yaml:"enabled"`
		ListenAddr string `yaml:"listen_addr"`
		Path       string `yaml:"path"`
		PublicHost string `yaml:"public_host"` // Public hostname for clients (e.g., vpn.example.com)
		TLSCert    string `yaml:"tls_cert"`
		TLSKey     string `yaml:"tls_key"`
	} `yaml:"tunnel"`
	NAT struct {
		Enabled    bool `yaml:"enabled"`
		Masquerade []struct {
			Interface string `yaml:"interface"`
		} `yaml:"masquerade"`
		SNAT []struct {
			Source      string `yaml:"source"`
			Destination string `yaml:"destination"`
			Interface   string `yaml:"interface"`
			ToSource    string `yaml:"to_source"`
		} `yaml:"snat"`
		DNAT []struct {
			Interface     string `yaml:"interface"`
			Protocol      string `yaml:"protocol"`
			Port          int    `yaml:"port"`
			ToDestination string `yaml:"to_destination"`
		} `yaml:"dnat"`
	} `yaml:"nat"`
}

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	initDB := flag.Bool("init-db", false, "Initialize database with default data")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("wire-socket-server version %s\n", Version)
		return
	}

	log.Printf("WireSocket Server %s", Version)

	// Load configuration
	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Setup logging based on config
	setupLogging(config.Server.LogLevel)

	// Initialize database
	db, err := database.NewDB(config.Database.Path)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	log.Println("Database initialized successfully")

	// Determine WireGuard mode
	wgMode := wireguard.Mode(config.WireGuard.Mode)
	if wgMode == "" {
		wgMode = wireguard.ModeKernel // Default to kernel mode
	}

	// Initialize WireGuard manager
	wgManager, err := wireguard.NewManagerWithConfig(wireguard.ManagerConfig{
		DeviceName: config.WireGuard.DeviceName,
		Mode:       wgMode,
	})
	if err != nil {
		log.Fatalf("Failed to initialize WireGuard manager: %v", err)
	}
	defer wgManager.Close()

	log.Printf("WireGuard manager initialized successfully (mode: %s)", wgMode)

	// Generate or load WireGuard server keys
	// Priority: 1. config.yaml, 2. existing wg config file, 3. generate new
	privateKey, publicKey := config.WireGuard.PrivateKey, config.WireGuard.PublicKey
	if privateKey == "" || publicKey == "" {
		// Try to load from existing WireGuard config file
		existingConfig, err := wgManager.LoadConfigFile()
		if err == nil && existingConfig != nil && existingConfig.PrivateKey != "" {
			log.Println("Loading WireGuard keys from existing config file...")
			privateKey = existingConfig.PrivateKey
			// Derive public key from private key
			publicKey, err = derivePublicKey(privateKey)
			if err != nil {
				log.Printf("Warning: failed to derive public key: %v, generating new keys", err)
				privateKey, publicKey, err = wireguard.GenerateKeyPair()
				if err != nil {
					log.Fatalf("Failed to generate key pair: %v", err)
				}
			} else {
				log.Printf("Loaded existing keys - Public: %s", publicKey)
			}
		} else {
			log.Println("Generating new WireGuard key pair...")
			privateKey, publicKey, err = wireguard.GenerateKeyPair()
			if err != nil {
				log.Fatalf("Failed to generate key pair: %v", err)
			}
			log.Printf("Generated keys - Public: %s", publicKey)
			log.Println("TIP: Save these keys in config.yaml to persist across restarts")
		}
	}

	// Calculate server address BEFORE configuring device
	// (first usable IP in subnet, e.g., 10.250.2.0/24 -> 10.250.2.1/24)
	serverAddr, err := getServerAddress(config.WireGuard.Subnet)
	if err != nil {
		log.Fatalf("Failed to calculate server address: %v", err)
	}
	wgManager.SetAddress(serverAddr)
	log.Printf("WireGuard server address: %s", serverAddr)

	// Configure WireGuard device (must be after SetAddress so TUN gets the IP)
	if err := wgManager.ConfigureDevice(privateKey, config.WireGuard.ListenPort); err != nil {
		log.Fatalf("Failed to configure WireGuard device: %v", err)
	}

	log.Printf("WireGuard device %s configured successfully", config.WireGuard.DeviceName)

	// Load existing peers from config file (if any)
	if err := wgManager.LoadPeersFromConfig(); err != nil {
		log.Printf("Warning: failed to load peers from config file: %v", err)
	} else {
		existingConfig, _ := wgManager.LoadConfigFile()
		if existingConfig != nil && len(existingConfig.Peers) > 0 {
			log.Printf("Loaded %d peers from %s", len(existingConfig.Peers), wgManager.GetConfigPath())
		}
	}

	// Save initial config (creates file if not exists)
	if err := wgManager.SaveConfigFile(privateKey, config.WireGuard.Subnet, config.WireGuard.ListenPort); err != nil {
		log.Printf("Warning: failed to save initial config: %v", err)
	} else {
		log.Printf("WireGuard config persisted to %s", wgManager.GetConfigPath())
	}

	// Create default server entry in database
	if *initDB {
		if err := initializeDatabase(db, config, publicKey); err != nil {
			log.Fatalf("Failed to initialize database: %v", err)
		}
		log.Println("Database initialized with default data")
		return
	}

	// Ensure server record in database has the correct public key
	// This handles the case where keys are generated on startup
	if err := syncServerPublicKey(db, config, publicKey); err != nil {
		log.Printf("Warning: failed to sync server public key: %v", err)
	}

	// Initialize config generator
	configGen := wireguard.NewConfigGenerator(db, wgManager)

	// Initialize auth handler
	authHandler := auth.NewHandler(db, config.Auth.JWTSecret, config.Auth.AllowRegistration)

	// Set up Gin router
	engine := gin.Default()

	// Enable CORS
	engine.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Set up API routes
	// Build tunnel URL from public_host and path
	tunnelURL := ""
	if config.Tunnel.PublicHost != "" {
		path := config.Tunnel.Path
		if path == "" {
			path = "/"
		}
		tunnelURL = "wss://" + config.Tunnel.PublicHost + path
	}

	// Initialize NAT manager - load from database first, fallback to config
	natConfig := loadNATConfig(db, config)
	natManager := nat.NewManager(natConfig)
	if err := natManager.Apply(); err != nil {
		log.Printf("Warning: failed to apply NAT rules: %v", err)
	}

	// Initialize admin handler
	adminHandler := api.NewAdminHandler(db, natManager, config.WireGuard.DeviceName)

	apiRouter := api.NewRouter(authHandler, adminHandler, db, configGen, tunnelURL, config.WireGuard.Subnet)
	apiRouter.SetupRoutes(engine)

	// Setup admin UI routes
	admin.SetupRoutes(engine)

	// Start server
	log.Printf("Starting VPN server on %s", config.Server.Address)
	log.Printf("WireGuard endpoint: %s", config.WireGuard.Endpoint)
	log.Printf("VPN subnet: %s", config.WireGuard.Subnet)

	// Start built-in tunnel server if enabled
	var tunnelServer *tunnel.Server
	if config.Tunnel.Enabled {
		targetAddr := fmt.Sprintf("127.0.0.1:%d", config.WireGuard.ListenPort)
		tunnelServer = tunnel.NewServer(tunnel.Config{
			ListenAddr: config.Tunnel.ListenAddr,
			TargetAddr: targetAddr,
			PathPrefix: config.Tunnel.Path,
			TLSCert:    config.Tunnel.TLSCert,
			TLSKey:     config.Tunnel.TLSKey,
		})

		if err := tunnelServer.StartAsync(); err != nil {
			log.Fatalf("Failed to start tunnel server: %v", err)
		}
		defer tunnelServer.Stop()

		protocol := "WS"
		if config.Tunnel.TLSCert != "" {
			protocol = "WSS"
		}
		log.Printf("Built-in tunnel server started on %s (%s -> %s)", config.Tunnel.ListenAddr, protocol, targetAddr)
	} else {
		log.Println("")
		log.Println("Built-in tunnel disabled. Make sure wstunnel server is running:")
		log.Println("  wstunnel server wss://0.0.0.0:443 --restrict-to 127.0.0.1:51820")
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("\nShutting down...")
		natManager.Cleanup()
		if tunnelServer != nil {
			tunnelServer.Stop()
		}
		wgManager.Close()
		os.Exit(0)
	}()

	log.Println("")
	log.Println("Server is ready!")

	if config.Server.TLS != nil {
		if err := engine.RunTLS(config.Server.Address, config.Server.TLS.CertFile, config.Server.TLS.KeyFile); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	} else {
		if err := engine.Run(config.Server.Address); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
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

func initializeDatabase(db *database.DB, config *Config, publicKey string) error {
	// Create default server
	server := &database.Server{
		Name:       "Default Server",
		Endpoint:   config.WireGuard.Endpoint,
		PublicKey:  publicKey,
		PrivateKey: config.WireGuard.PrivateKey, // Should be encrypted in production
		ListenPort: config.WireGuard.ListenPort,
		Subnet:     config.WireGuard.Subnet,
		DNS:        config.WireGuard.DNS,
	}

	if err := db.Create(server).Error; err != nil {
		return fmt.Errorf("failed to create default server: %w", err)
	}

	// Create default admin user
	// Password: admin123 (change this immediately!)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	adminUser := &database.User{
		Username:     "admin",
		Email:        "admin@example.com",
		PasswordHash: string(hashedPassword),
		IsActive:     true,
		IsAdmin:      true,
	}

	if err := db.Create(adminUser).Error; err != nil {
		return fmt.Errorf("failed to create admin user: %w", err)
	}

	log.Println("Created default admin user:")
	log.Println("  Username: admin")
	log.Println("  Password: admin123")
	log.Println("  *** Please change this password immediately! ***")

	return nil
}

// syncServerPublicKey updates the server record in the database with the current public key
// This ensures the database always has the key that matches the running WireGuard instance
func syncServerPublicKey(db *database.DB, config *Config, publicKey string) error {
	var server database.Server
	if err := db.First(&server).Error; err != nil {
		return fmt.Errorf("no server record found: %w", err)
	}

	if server.PublicKey != publicKey {
		log.Printf("Updating server public key in database (was: %s..., now: %s...)",
			server.PublicKey[:8], publicKey[:8])
		server.PublicKey = publicKey
		server.PrivateKey = config.WireGuard.PrivateKey
		if err := db.Save(&server).Error; err != nil {
			return fmt.Errorf("failed to update server: %w", err)
		}
		log.Println("Server public key updated in database")
	}

	return nil
}

// derivePublicKey derives the public key from a WireGuard private key
func derivePublicKey(privateKeyStr string) (string, error) {
	privateKey, err := wgtypes.ParseKey(privateKeyStr)
	if err != nil {
		return "", fmt.Errorf("invalid private key: %w", err)
	}
	return privateKey.PublicKey().String(), nil
}

// getServerAddress returns the first usable IP in a subnet as the server address
// e.g., "10.250.2.0/24" -> "10.250.2.1/24"
func getServerAddress(subnet string) (string, error) {
	ip, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return "", fmt.Errorf("invalid subnet %s: %w", subnet, err)
	}

	// Get the network address
	networkIP := ip.Mask(ipNet.Mask)

	// Convert to 4-byte representation for IPv4
	ip4 := networkIP.To4()
	if ip4 == nil {
		return "", fmt.Errorf("only IPv4 subnets are supported")
	}

	// Increment to get first usable IP (x.x.x.1)
	ip4[3]++

	// Get prefix length
	ones, _ := ipNet.Mask.Size()

	return fmt.Sprintf("%s/%d", ip4.String(), ones), nil
}

// loadNATConfig loads NAT configuration from database, falling back to config.yaml if database is empty
func loadNATConfig(db *database.DB, config *Config) nat.Config {
	natConfig := nat.Config{
		Enabled: config.NAT.Enabled,
	}

	// Try to load from database first
	var rules []database.NATRule
	if err := db.Where("enabled = ?", true).Find(&rules).Error; err == nil && len(rules) > 0 {
		log.Printf("Loading NAT rules from database (%d rules)", len(rules))
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
		return natConfig
	}

	// Fall back to config.yaml
	log.Println("Loading NAT rules from config.yaml (no rules in database)")
	for _, m := range config.NAT.Masquerade {
		natConfig.Masquerade = append(natConfig.Masquerade, nat.MasqueradeRule{
			Interface: m.Interface,
		})
	}
	for _, s := range config.NAT.SNAT {
		natConfig.SNAT = append(natConfig.SNAT, nat.SNATRule{
			Source:      s.Source,
			Destination: s.Destination,
			Interface:   s.Interface,
			ToSource:    s.ToSource,
		})
	}
	for _, d := range config.NAT.DNAT {
		natConfig.DNAT = append(natConfig.DNAT, nat.DNATRule{
			Interface:     d.Interface,
			Protocol:      d.Protocol,
			Port:          d.Port,
			ToDestination: d.ToDestination,
		})
	}

	return natConfig
}
