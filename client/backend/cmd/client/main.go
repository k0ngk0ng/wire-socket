package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"wire-socket-client/internal/api"
	"wire-socket-client/internal/connection"

	"github.com/kardianos/service"
)

// Version is set at build time via -ldflags
var Version = "dev"

var logger service.Logger

// Default port and range to try
const (
	DefaultPort = 41945
	MaxPortTries = 10
)

// Program implements the service.Interface
type Program struct {
	apiServer *api.Server
	connMgr   *connection.Manager
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
		dir = os.TempDir()
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
	// Initialize connection manager
	var err error
	p.connMgr, err = connection.NewManager()
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
	p.apiServer = api.NewServer(p.connMgr, addr)
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

func main() {
	// Parse command line flags
	svcFlag := flag.String("service", "", "Control the system service: install, uninstall, start, stop, restart")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("wire-socket-client version %s\n", Version)
		return
	}

	// Service configuration
	svcConfig := &service.Config{
		Name:        "WireSocketClient",
		DisplayName: "WireSocket Client Service",
		Description: "Manages VPN connections with WireGuard and wstunnel",
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
