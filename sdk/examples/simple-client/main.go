// Example: Simple CLI VPN client using WireSocket SDK
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	sdk "github.com/k0ngk0ng/wire-socket/sdk"
)

func main() {
	// Parse flags
	server := flag.String("server", "", "VPN server address")
	username := flag.String("user", "", "Username")
	password := flag.String("pass", "", "Password")
	flag.Parse()

	if *server == "" || *username == "" || *password == "" {
		fmt.Println("Usage: simple-client -server <addr> -user <user> -pass <pass>")
		os.Exit(1)
	}

	// Create SDK client
	client, err := sdk.New(sdk.Options{
		StatsInterval: 5 * time.Second,
		Debug:         true,
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Register event handlers
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
			if event.Stats != nil {
				log.Printf("Stats: RX=%d bytes, TX=%d bytes",
					event.Stats.RxBytes, event.Stats.TxBytes)
			}
		}
	})

	// Connect
	config := sdk.DefaultConnectConfig()
	config.Server = *server
	config.Username = *username
	config.Password = *password
	config.AutoReconnect = true

	if err := client.Connect(config); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}

	// Wait for connection
	timeout := time.After(30 * time.Second)
	for {
		select {
		case <-timeout:
			log.Fatal("Connection timeout")
		default:
			if client.IsConnected() {
				goto connected
			}
			if client.GetState() == sdk.StateFailed {
				log.Fatalf("Connection failed: %s", client.GetStatus().Error)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

connected:
	log.Println("VPN connected. Press Ctrl+C to disconnect...")

	// Wait for signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	client.Disconnect()
}
