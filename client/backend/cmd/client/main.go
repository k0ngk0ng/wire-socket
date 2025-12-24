package main

import (
	"flag"
	"log"
	"os"
	"wire-socket-client/internal/api"
	"wire-socket-client/internal/connection"

	"github.com/kardianos/service"
)

var logger service.Logger

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

func (p *Program) run() {
	// Initialize connection manager
	var err error
	p.connMgr, err = connection.NewManager()
	if err != nil {
		logger.Errorf("Failed to create connection manager: %v", err)
		return
	}

	// Start local API server
	p.apiServer = api.NewServer(p.connMgr, ":41945")
	if err := p.apiServer.Start(); err != nil {
		logger.Errorf("Failed to start API server: %v", err)
		return
	}

	logger.Info("WireSocket Client Service started successfully")
	logger.Info("API server listening on localhost:41945")
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
	flag.Parse()

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
