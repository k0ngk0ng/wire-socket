// Example: Event-driven VPN client with custom UI simulation
package main

import (
	"fmt"
	"log"
	"time"

	sdk "github.com/k0ngk0ng/wire-socket/sdk"
)

// UIController simulates a UI that reacts to VPN events
type UIController struct {
	client *sdk.Client
}

func NewUIController() (*UIController, error) {
	client, err := sdk.New()
	if err != nil {
		return nil, err
	}

	ui := &UIController{client: client}
	ui.setupEventHandlers()
	return ui, nil
}

func (ui *UIController) setupEventHandlers() {
	ui.client.OnEvent(func(event sdk.Event) {
		switch event.Type {
		case sdk.EventConnecting:
			ui.showConnectingUI()

		case sdk.EventConnected:
			ui.showConnectedUI(event.Status)

		case sdk.EventDisconnected:
			ui.showDisconnectedUI()

		case sdk.EventReconnecting:
			ui.showReconnectingUI()

		case sdk.EventError:
			ui.showError(event.Error)

		case sdk.EventStatsUpdated:
			ui.updateStats(event.Stats)

		case sdk.EventRoutesChanged:
			ui.updateRoutes()
		}
	})
}

func (ui *UIController) showConnectingUI() {
	fmt.Println("\n┌─────────────────────────────────┐")
	fmt.Println("│  🔄 Connecting to VPN...        │")
	fmt.Println("└─────────────────────────────────┘")
}

func (ui *UIController) showConnectedUI(status *sdk.Status) {
	fmt.Println("\n┌─────────────────────────────────┐")
	fmt.Println("│  ✅ VPN Connected               │")
	fmt.Println("├─────────────────────────────────┤")
	fmt.Printf("│  Server: %-22s │\n", truncate(status.Server, 22))
	fmt.Printf("│  IP:     %-22s │\n", status.AssignedIP)
	fmt.Println("└─────────────────────────────────┘")
}

func (ui *UIController) showDisconnectedUI() {
	fmt.Println("\n┌─────────────────────────────────┐")
	fmt.Println("│  ⚪ VPN Disconnected            │")
	fmt.Println("└─────────────────────────────────┘")
}

func (ui *UIController) showReconnectingUI() {
	fmt.Println("\n┌─────────────────────────────────┐")
	fmt.Println("│  🔄 Reconnecting...             │")
	fmt.Println("└─────────────────────────────────┘")
}

func (ui *UIController) showError(err error) {
	fmt.Println("\n┌─────────────────────────────────┐")
	fmt.Println("│  ❌ Error                       │")
	fmt.Printf("│  %s\n", truncate(err.Error(), 30))
	fmt.Println("└─────────────────────────────────┘")
}

func (ui *UIController) updateStats(stats *sdk.Stats) {
	if stats == nil {
		return
	}
	fmt.Printf("\r  📊 RX: %s | TX: %s   ",
		formatBytes(stats.RxBytes),
		formatBytes(stats.TxBytes))
}

func (ui *UIController) updateRoutes() {
	routes := ui.client.GetRoutes()
	fmt.Printf("\n  Routes: %d active, %d excluded\n",
		len(routes.ActiveRoutes), len(routes.ExcludedRoutes))
}

func (ui *UIController) Connect(server, user, pass string) error {
	config := sdk.DefaultConnectConfig()
	config.Server = server
	config.Username = user
	config.Password = pass
	return ui.client.Connect(config)
}

func (ui *UIController) Disconnect() error {
	return ui.client.Disconnect()
}

func (ui *UIController) Close() {
	ui.client.Close()
}

// Helper functions
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func main() {
	ui, err := NewUIController()
	if err != nil {
		log.Fatalf("Failed to create UI: %v", err)
	}
	defer ui.Close()

	fmt.Println("WireSocket SDK - Event-Driven UI Example")
	fmt.Println("=========================================")
	fmt.Println()
	fmt.Println("This example demonstrates how to build a reactive UI")
	fmt.Println("that responds to VPN events.")
	fmt.Println()
	fmt.Println("To use:")
	fmt.Println("  ui.Connect(\"https://vpn.example.com\", \"user\", \"pass\")")
	fmt.Println("  ui.Disconnect()")
	fmt.Println()

	// Demo: simulate events
	fmt.Println("Simulating events...")
	time.Sleep(1 * time.Second)

	ui.showDisconnectedUI()
	time.Sleep(1 * time.Second)

	ui.showConnectingUI()
	time.Sleep(2 * time.Second)

	ui.showConnectedUI(&sdk.Status{
		Server:     "vpn.example.com",
		AssignedIP: "10.0.0.5",
	})

	// Simulate stats updates
	for i := 0; i < 5; i++ {
		time.Sleep(1 * time.Second)
		ui.updateStats(&sdk.Stats{
			RxBytes: uint64(i) * 1024 * 100,
			TxBytes: uint64(i) * 1024 * 50,
		})
	}

	fmt.Println()
	ui.showDisconnectedUI()
}
