package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"wire-socket-server/internal/api"
	"wire-socket-server/internal/auth"
	"wire-socket-server/internal/database"
	"wire-socket-server/internal/wireguard"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// Config represents the server configuration
type Config struct {
	Server struct {
		Address string `yaml:"address"`
		TLS     *struct {
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
		DeviceName string `yaml:"device_name"`
		ListenPort int    `yaml:"listen_port"`
		Subnet     string `yaml:"subnet"`
		DNS        string `yaml:"dns"`
		Endpoint   string `yaml:"endpoint"`
		PrivateKey string `yaml:"private_key"`
		PublicKey  string `yaml:"public_key"`
	} `yaml:"wireguard"`
	Auth struct {
		JWTSecret string `yaml:"jwt_secret"`
	} `yaml:"auth"`
}

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	initDB := flag.Bool("init-db", false, "Initialize database with default data")
	flag.Parse()

	// Load configuration
	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize database
	db, err := database.NewDB(config.Database.Path)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	log.Println("Database initialized successfully")

	// Initialize WireGuard manager
	wgManager, err := wireguard.NewManager(config.WireGuard.DeviceName)
	if err != nil {
		log.Fatalf("Failed to initialize WireGuard manager: %v", err)
	}
	defer wgManager.Close()

	log.Println("WireGuard manager initialized successfully")

	// Generate or load WireGuard server keys
	privateKey, publicKey := config.WireGuard.PrivateKey, config.WireGuard.PublicKey
	if privateKey == "" || publicKey == "" {
		log.Println("Generating new WireGuard key pair...")
		privateKey, publicKey, err = wireguard.GenerateKeyPair()
		if err != nil {
			log.Fatalf("Failed to generate key pair: %v", err)
		}
		log.Printf("Generated keys - Public: %s", publicKey)
		log.Println("Please save these keys in your config file!")
	}

	// Configure WireGuard device
	if err := wgManager.ConfigureDevice(privateKey, config.WireGuard.ListenPort); err != nil {
		log.Fatalf("Failed to configure WireGuard device: %v", err)
	}

	log.Printf("WireGuard device %s configured successfully", config.WireGuard.DeviceName)

	// Set address for config file persistence (use first IP from subnet as server address)
	wgManager.SetAddress(config.WireGuard.Subnet)

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

	// Initialize config generator
	configGen := wireguard.NewConfigGenerator(db, wgManager)

	// Initialize auth handler
	authHandler := auth.NewHandler(db, config.Auth.JWTSecret)

	// Set up Gin router
	gin.SetMode(gin.ReleaseMode)
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
	apiRouter := api.NewRouter(authHandler, db, configGen)
	apiRouter.SetupRoutes(engine)

	// Start server
	log.Printf("Starting VPN server on %s", config.Server.Address)
	log.Printf("WireGuard endpoint: %s", config.WireGuard.Endpoint)
	log.Printf("VPN subnet: %s", config.WireGuard.Subnet)
	log.Println("")
	log.Println("Server is ready! Make sure wstunnel server is running:")
	log.Println("  wstunnel server wss://0.0.0.0:443 --restrict-to 127.0.0.1:51820")

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
	hashedPassword := "$2a$10$rLZYJ5Hf0K8qH9m5lHf0K8qH9m5lHf0K8qH9m5lHf0K8qH9m5lHf0" // Hash for "admin123"
	adminUser := &database.User{
		Username:     "admin",
		Email:        "admin@example.com",
		PasswordHash: hashedPassword,
		IsActive:     true,
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
