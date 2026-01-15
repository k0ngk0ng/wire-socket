package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"wire-socket-client/internal/api"
	"wire-socket-client/internal/sdkadapter"

	sdk "github.com/k0ngk0ng/wire-socket/sdk"
	"github.com/kardianos/service"
)

// Version is set at build time via -ldflags
var Version = "dev"

var logger service.Logger

// Default port and range to try
const (
	DefaultPort  = 41945
	MaxPortTries = 10
)

// CLIConfig represents the configuration file for CLI mode
type CLIConfig struct {
	Server   string `json:"server"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// Program implements the service.Interface
type Program struct {
	apiServer *api.Server
	connMgr   *sdkadapter.Manager
}

func (p *Program) Start(s service.Service) error {
	logger.Info("Starting WireSocket Client Service...")
	go p.run()
	return nil
}

// getPortFilePath returns the path to the port file
func getPortFilePath() string {
	var dir string
	switch runtime.GOOS {
	case "darwin":
		dir = "/tmp"
	case "linux":
		dir = "/tmp"
	case "windows":
		// Use ProgramData which is accessible to both SYSTEM and user accounts
		dir = os.Getenv("ProgramData")
		if dir == "" {
			dir = "C:\\ProgramData"
		}
		dir = filepath.Join(dir, "WireSocket")
		// Create directory if it doesn't exist
		os.MkdirAll(dir, 0755)
	default:
		dir = "/tmp"
	}
	return filepath.Join(dir, "wiresocket-port")
}

// findAvailablePort tries ports starting from DefaultPort
func findAvailablePort() (int, error) {
	for i := 0; i < MaxPortTries; i++ {
		port := DefaultPort + i
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			listener.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available port found in range %d-%d", DefaultPort, DefaultPort+MaxPortTries-1)
}

// writePortFile writes the selected port to a file
func writePortFile(port int) error {
	path := getPortFilePath()
	return os.WriteFile(path, []byte(fmt.Sprintf("%d", port)), 0644)
}

func (p *Program) run() {
	// Initialize SDK-backed connection manager
	var err error
	p.connMgr, err = sdkadapter.NewManager()
	if err != nil {
		logger.Errorf("Failed to create connection manager: %v", err)
		return
	}

	// Find available port
	port, err := findAvailablePort()
	if err != nil {
		logger.Errorf("Failed to find available port: %v", err)
		return
	}

	// Write port to file so frontend can find it
	if err := writePortFile(port); err != nil {
		logger.Warningf("Failed to write port file: %v", err)
		// Continue anyway, frontend will try default port
	}

	// Start local API server
	addr := fmt.Sprintf(":%d", port)
	p.apiServer = api.NewServer(p.connMgr, addr, Version)
	if err := p.apiServer.Start(); err != nil {
		logger.Errorf("Failed to start API server: %v", err)
		return
	}

	logger.Infof("WireSocket Client Service started successfully")
	logger.Infof("API server listening on localhost:%d", port)
}

func (p *Program) Stop(s service.Service) error {
	logger.Info("Stopping WireSocket Client Service...")

	// Stop API server
	if p.apiServer != nil {
		p.apiServer.Stop()
	}

	// Disconnect if connected
	if p.connMgr != nil {
		p.connMgr.Disconnect()
		p.connMgr.Close()
	}

	logger.Info("WireSocket Client Service stopped")
	return nil
}

// getDefaultConfigPath returns the default path for CLI config file
func getDefaultConfigPath() string {
	switch runtime.GOOS {
	case "linux":
		return "/etc/wire-socket/client.json"
	case "darwin":
		return "/etc/wire-socket/client.json"
	default:
		return filepath.Join(os.Getenv("ProgramData"), "WireSocket", "client.json")
	}
}

// loadCLIConfig loads configuration from a JSON file
func loadCLIConfig(path string) (*CLIConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config CLIConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// runCLIConnect runs the VPN connection in CLI mode using SDK
func runCLIConnect(server, username, password string, daemon bool) error {
	log.Printf("WireSocket CLI Client v%s", Version)
	log.Printf("Connecting to %s as %s...", server, username)

	// Create SDK client directly for CLI mode
	client, err := sdk.New(sdk.Options{
		StatsInterval: 60 * time.Second,
		Debug:         false,
	})
	if err != nil {
		return fmt.Errorf("failed to create SDK client: %w", err)
	}
	defer client.Close()

	// Register event handler
	client.OnEvent(func(event sdk.Event) {
		switch event.Type {
		case sdk.EventConnecting:
			log.Println("Connecting...")
		case sdk.EventConnected:
			log.Printf("Connected! IP: %s", event.Status.AssignedIP)
		case sdk.EventDisconnected:
			log.Println("Disconnected")
		case sdk.EventReconnecting:
			log.Println("Connection lost, reconnecting...")
		case sdk.EventError:
			log.Printf("Error: %v", event.Error)
		case sdk.EventStatsUpdated:
			if event.Stats != nil && client.IsConnected() {
				log.Printf("Stats: RX=%d bytes, TX=%d bytes", event.Stats.RxBytes, event.Stats.TxBytes)
			}
		}
	})

	// Connect
	config := sdk.DefaultConnectConfig()
	config.Server = server
	config.Username = username
	config.Password = password
	config.AutoReconnect = daemon

	if err := client.Connect(config); err != nil {
		return fmt.Errorf("failed to initiate connection: %w", err)
	}

	// Wait for connection to complete
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			client.Disconnect()
			return fmt.Errorf("connection timed out")
		case <-ticker.C:
			state := client.GetState()
			switch state {
			case sdk.StateConnected:
				goto connected
			case sdk.StateFailed:
				return fmt.Errorf("connection failed: %s", client.GetStatus().Error)
			}
		}
	}

connected:
	if !daemon {
		log.Println("VPN connected. Press Ctrl+C to disconnect...")
	} else {
		log.Println("VPN connected in daemon mode.")
	}

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for signal
	sig := <-sigChan
	log.Printf("Received signal %v, disconnecting...", sig)
	client.Disconnect()
	log.Println("Disconnected. Goodbye!")
	return nil
}

// printCLIUsage prints CLI usage information
func printCLIUsage() {
	fmt.Println("WireSocket Client - VPN Client")
	fmt.Printf("Version: %s\n\n", Version)
	fmt.Println("Usage:")
	fmt.Println("  wire-socket-client [options]")
	fmt.Println()
	fmt.Println("Service Mode (for GUI frontend):")
	fmt.Println("  wire-socket-client                     Run as service (for systemd/launchd)")
	fmt.Println("  wire-socket-client -service install    Install as system service")
	fmt.Println("  wire-socket-client -service uninstall  Uninstall system service")
	fmt.Println("  wire-socket-client -service start      Start the service")
	fmt.Println("  wire-socket-client -service stop       Stop the service")
	fmt.Println()
	fmt.Println("CLI Mode (direct VPN connection):")
	fmt.Println("  wire-socket-client connect -server <addr> -user <user> -pass <pass>")
	fmt.Println("  wire-socket-client connect -config /path/to/config.json")
	fmt.Println("  wire-socket-client connect -config /path/to/config.json -daemon")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -version        Show version and exit")
	fmt.Println("  -help           Show this help message")
	fmt.Println()
	fmt.Println("Config file format (JSON):")
	fmt.Println("  {")
	fmt.Println("    \"server\": \"https://vpn.example.com\",")
	fmt.Println("    \"username\": \"user\",")
	fmt.Println("    \"password\": \"pass\"")
	fmt.Println("  }")
	fmt.Println()
	fmt.Println("Systemd Service (CLI mode):")
	fmt.Println("  1. Create config file: /etc/wire-socket/client.json")
	fmt.Println("  2. Install service: wire-socket-client -service install")
	fmt.Println("  3. Or use provided systemd unit: wire-socket-client.service")
}

func main() {
	// Check for subcommand first
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "connect":
			runConnectCommand()
			return
		case "help", "-help", "--help":
			printCLIUsage()
			return
		}
	}

	// Parse command line flags for service mode
	svcFlag := flag.String("service", "", "Control the system service: install, uninstall, start, stop, restart")
	showVersion := flag.Bool("version", false, "Show version and exit")
	showHelp := flag.Bool("help", false, "Show help message")
	flag.Parse()

	if *showHelp {
		printCLIUsage()
		return
	}

	if *showVersion {
		fmt.Printf("wire-socket-client version %s\n", Version)
		return
	}

	// Get executable directory (for wintun.dll on Windows)
	exeDir := ""
	if exePath, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exePath)
		// Also change current directory as fallback
		os.Chdir(exeDir)
	}

	// Service configuration
	svcConfig := &service.Config{
		Name:             "WireSocketClient",
		DisplayName:      "WireSocket Client Service",
		Description:      "Manages VPN connections with WireGuard and wstunnel",
		WorkingDirectory: exeDir, // Set working directory for service (needed for wintun.dll)
		Option: service.KeyValue{
			// macOS launchd: start service at system boot
			"RunAtLoad": true,
			// Windows: start automatically
			"StartType": "automatic",
		},
	}

	prg := &Program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatalf("Failed to create service: %v", err)
	}

	logger, err = s.Logger(nil)
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}

	// Handle service control commands
	if *svcFlag != "" {
		err := service.Control(s, *svcFlag)
		if err != nil {
			log.Printf("Service control error: %v", err)
			os.Exit(1)
		}
		log.Printf("Service %s completed successfully", *svcFlag)
		return
	}

	// Run the service
	err = s.Run()
	if err != nil {
		logger.Error(err)
	}
}

// runConnectCommand handles the "connect" subcommand
func runConnectCommand() {
	connectCmd := flag.NewFlagSet("connect", flag.ExitOnError)
	server := connectCmd.String("server", "", "Server address (e.g., https://vpn.example.com)")
	username := connectCmd.String("user", "", "Username")
	password := connectCmd.String("pass", "", "Password")
	configFile := connectCmd.String("config", "", "Path to config file (JSON)")
	daemon := connectCmd.Bool("daemon", false, "Run in daemon mode (auto-reconnect)")

	connectCmd.Parse(os.Args[2:])

	var cfg *CLIConfig

	// Load from config file if specified
	if *configFile != "" {
		var err error
		cfg, err = loadCLIConfig(*configFile)
		if err != nil {
			log.Fatalf("Error loading config: %v", err)
		}
	} else if *server == "" {
		// Try default config path
		defaultPath := getDefaultConfigPath()
		if _, err := os.Stat(defaultPath); err == nil {
			cfg, _ = loadCLIConfig(defaultPath)
		}
	}

	// Override with command line arguments
	if cfg == nil {
		cfg = &CLIConfig{}
	}
	if *server != "" {
		cfg.Server = *server
	}
	if *username != "" {
		cfg.Username = *username
	}
	if *password != "" {
		cfg.Password = *password
	}

	// Validate
	if cfg.Server == "" || cfg.Username == "" || cfg.Password == "" {
		log.Fatal("Error: server, username, and password are required")
	}

	// Run connection
	if err := runCLIConnect(cfg.Server, cfg.Username, cfg.Password, *daemon); err != nil {
		log.Fatalf("Error: %v", err)
	}
}
