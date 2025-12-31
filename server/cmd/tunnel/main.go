package main

import (
	"flag"
	"log"
	"os"
	"strconv"
	"time"

	"wire-socket-server/internal/database"
	"wire-socket-server/internal/nat"
	"wire-socket-server/internal/tunnel"
	"wire-socket-server/internal/tunnelservice"
	"wire-socket-server/internal/wireguard"

	"github.com/gin-gonic/gin"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"gopkg.in/yaml.v3"
)

// Config represents the tunnel service configuration
type Config struct {
	Tunnel struct {
		ID          string `yaml:"id"`           // Unique tunnel ID (e.g., "hk-01")
		Name        string `yaml:"name"`         // Display name (e.g., "Hong Kong")
		Region      string `yaml:"region"`       // Region code
		Token       string `yaml:"token"`        // Secret token for auth service
		MasterToken string `yaml:"master_token"` // For initial registration
	} `yaml:"tunnel"`
	Auth struct {
		URL string `yaml:"url"` // Auth service URL
	} `yaml:"auth"`
	Server struct {
		Address string `yaml:"address"`
	} `yaml:"server"`
	Database struct {
		Path string `yaml:"path"`
	} `yaml:"database"`
	WireGuard struct {
		DeviceName string `yaml:"device_name"`
		ListenPort int    `yaml:"listen_port"`
		Subnet     string `yaml:"subnet"`
		DNS        string `yaml:"dns"`
		Endpoint   string `yaml:"endpoint"`
		PrivateKey string `yaml:"private_key"`
		PublicKey  string `yaml:"public_key"`
		Mode       string `yaml:"mode"` // "userspace" or "kernel"
	} `yaml:"wireguard"`
	WebSocketTunnel struct {
		Enabled    bool   `yaml:"enabled"`
		ListenAddr string `yaml:"listen_addr"`
		PublicHost string `yaml:"public_host"`
		Path       string `yaml:"path"`
		TLSCert    string `yaml:"tls_cert"`
		TLSKey     string `yaml:"tls_key"`
	} `yaml:"ws_tunnel"`
	PeerCleanup struct {
		Enabled  *bool `yaml:"enabled"`  // Enable peer cleanup (default: true)
		Timeout  int   `yaml:"timeout"`  // Seconds before inactive peer is removed (default: 180)
		Interval int   `yaml:"interval"` // Seconds between cleanup checks (default: 30)
	} `yaml:"peer_cleanup"`
}

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	genKey := flag.Bool("gen-key", false, "Generate WireGuard keys")
	register := flag.Bool("register", false, "Register with auth service")
	flag.Parse()

	// Load config
	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Generate keys if requested
	if *genKey {
		privateKey, err := wgtypes.GeneratePrivateKey()
		if err != nil {
			log.Fatalf("Failed to generate key: %v", err)
		}
		log.Printf("Private Key: %s", privateKey.String())
		log.Printf("Public Key:  %s", privateKey.PublicKey().String())
		return
	}

	// Initialize database (auto-migrate on startup)
	db, err := database.NewTunnelDB(config.Database.Path)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	if err := db.AutoMigrate(); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	// Parse private key
	privateKey, err := wgtypes.ParseKey(config.WireGuard.PrivateKey)
	if err != nil {
		log.Fatalf("Failed to parse private key: %v", err)
	}
	publicKey := privateKey.PublicKey().String()

	// Initialize WireGuard
	wgMode := wireguard.ModeUserspace
	if config.WireGuard.Mode == "kernel" {
		wgMode = wireguard.ModeKernel
	}

	wgManager, err := wireguard.NewManagerWithConfig(wireguard.ManagerConfig{
		DeviceName: config.WireGuard.DeviceName,
		Mode:       wgMode,
	})
	if err != nil {
		log.Fatalf("Failed to create WireGuard manager: %v", err)
	}

	// Set interface address
	wgManager.SetAddress(config.WireGuard.Subnet)

	// Configure WireGuard device
	if err := wgManager.ConfigureDevice(privateKey.String(), config.WireGuard.ListenPort); err != nil {
		log.Fatalf("Failed to configure WireGuard: %v", err)
	}

	log.Printf("WireGuard device %s configured", config.WireGuard.DeviceName)

	// Initialize NAT manager
	natManager := nat.NewManager(nat.Config{Enabled: false})

	// Build auth config
	tunnelURL := buildTunnelURL(config)
	authConfig := tunnelservice.AuthConfig{
		AuthURL:         config.Auth.URL,
		TunnelID:        config.Tunnel.ID,
		TunnelToken:     config.Tunnel.Token,
		TunnelURL:       tunnelURL,
		Subnet:          config.WireGuard.Subnet,
		ServerPublicKey: publicKey,
		Endpoint:        config.WireGuard.Endpoint,
		DNS:             []string{config.WireGuard.DNS},
	}

	// Register with auth service if requested
	if *register {
		tempHandler := tunnelservice.NewAuthHandler(db, wgManager, authConfig)
		internalURL := "http://" + config.Server.Address
		if err := tempHandler.RegisterWithAuth(config.Tunnel.Name, tunnelURL, internalURL, config.Tunnel.Region, config.Tunnel.MasterToken); err != nil {
			log.Fatalf("Failed to register with auth service: %v", err)
		}
		log.Println("Successfully registered with auth service")
		return
	}

	// Setup Gin
	gin.SetMode(gin.ReleaseMode)
	engine := gin.Default()

	// Setup API routes
	router := tunnelservice.NewRouter(db, wgManager, natManager, authConfig, config.WireGuard.DeviceName)
	router.SetupRoutes(engine)

	// Start peer cleanup service (enabled by default)
	// nil = not configured (default true), false = explicitly disabled
	if config.PeerCleanup.Enabled == nil || *config.PeerCleanup.Enabled {
		cleanupConfig := tunnelservice.CleanupConfig{
			Timeout:  time.Duration(config.PeerCleanup.Timeout) * time.Second,
			Interval: time.Duration(config.PeerCleanup.Interval) * time.Second,
		}
		// Use defaults if not specified
		if cleanupConfig.Timeout == 0 {
			cleanupConfig.Timeout = 3 * time.Minute
		}
		if cleanupConfig.Interval == 0 {
			cleanupConfig.Interval = 30 * time.Second
		}
		peerCleanup := tunnelservice.NewPeerCleanup(db, wgManager, cleanupConfig)
		peerCleanup.Start()
		defer peerCleanup.Stop()
	}

	// Serve admin UI
	engine.Static("/admin", "./internal/tunnelservice/admin/static")

	// Start WebSocket tunnel if enabled
	if config.WebSocketTunnel.Enabled {
		go startTunnel(config, wgManager)
	}

	// Start server
	log.Printf("Starting tunnel server on %s", config.Server.Address)
	log.Printf("Tunnel ID: %s", config.Tunnel.ID)
	log.Printf("Auth service: %s", config.Auth.URL)
	if err := engine.Run(config.Server.Address); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
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

	// Set defaults
	if config.Server.Address == "" {
		config.Server.Address = ":8080"
	}
	if config.Database.Path == "" {
		config.Database.Path = "tunnel.db"
	}
	if config.WireGuard.DeviceName == "" {
		config.WireGuard.DeviceName = "wg0"
	}
	if config.WireGuard.ListenPort == 0 {
		config.WireGuard.ListenPort = 51820
	}
	if config.WireGuard.Mode == "" {
		config.WireGuard.Mode = "userspace"
	}

	return &config, nil
}

func buildTunnelURL(config *Config) string {
	scheme := "ws"
	if config.WebSocketTunnel.TLSCert != "" {
		scheme = "wss"
	}
	path := config.WebSocketTunnel.Path
	if path == "" {
		path = "/"
	}
	return scheme + "://" + config.WebSocketTunnel.PublicHost + path
}

func startTunnel(config *Config, wgManager *wireguard.Manager) {
	tunnelConfig := tunnel.Config{
		ListenAddr: config.WebSocketTunnel.ListenAddr,
		TargetAddr: "127.0.0.1:" + strconv.Itoa(config.WireGuard.ListenPort),
		PathPrefix: config.WebSocketTunnel.Path,
		TLSCert:    config.WebSocketTunnel.TLSCert,
		TLSKey:     config.WebSocketTunnel.TLSKey,
	}

	server := tunnel.NewServer(tunnelConfig)
	log.Printf("Starting WebSocket tunnel on %s", config.WebSocketTunnel.ListenAddr)
	if err := server.Start(); err != nil {
		log.Printf("WebSocket tunnel error: %v", err)
	}
}
