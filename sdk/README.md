# WireSocket SDK

Go SDK for building custom WireSocket VPN clients and servers.

## Features

- **Client SDK**: Full-featured VPN client with event-driven API
- **Server SDK**: WireGuard VPN server with peer management
- **WireGuard Backends**: Kernel and userspace implementations
- **Mobile Support**: iOS and Android bindings via gomobile
- **Cross-Platform**: Linux, macOS, Windows, iOS, Android

## Installation

```bash
go get github.com/k0ngk0ng/wire-socket/sdk
```

## Quick Start - Client

```go
package main

import (
    "log"
    "time"

    sdk "github.com/k0ngk0ng/wire-socket/sdk"
)

func main() {
    // Create client
    client, err := sdk.New()
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Register event handler
    client.OnEvent(func(event sdk.Event) {
        switch event.Type {
        case sdk.EventConnected:
            log.Printf("Connected! IP: %s", event.Status.AssignedIP)
        case sdk.EventDisconnected:
            log.Println("Disconnected")
        case sdk.EventError:
            log.Printf("Error: %v", event.Error)
        }
    })

    // Connect
    config := sdk.DefaultConnectConfig()
    config.Server = "https://vpn.example.com"
    config.Username = "user"
    config.Password = "pass"

    if err := client.Connect(config); err != nil {
        log.Fatal(err)
    }

    // Wait for connection
    time.Sleep(10 * time.Second)

    // Check status
    status := client.GetStatus()
    log.Printf("Status: %s, IP: %s", status.State, status.AssignedIP)

    // Disconnect
    client.Disconnect()
}
```

## API Reference

### Creating a Client

```go
// With default options
client, err := sdk.New()

// With custom options
client, err := sdk.New(sdk.Options{
    ConfigDir:     "/path/to/config",
    StatsInterval: 5 * time.Second,
    Debug:         true,
    Logger:        log.Printf,
})
```

### Connection

```go
// Connect with config
config := sdk.DefaultConnectConfig()
config.Server = "https://vpn.example.com"
config.Username = "user"
config.Password = "pass"
config.AutoReconnect = true
config.ExcludedRoutes = []string{"192.168.1.0/24"}

err := client.Connect(config)

// Disconnect
err := client.Disconnect()

// Check state
if client.IsConnected() {
    // ...
}

state := client.GetState() // StateDisconnected, StateConnecting, StateConnected, etc.
```

### Status and Stats

```go
// Get current status
status := client.GetStatus()
fmt.Printf("State: %s\n", status.State)
fmt.Printf("Server: %s\n", status.Server)
fmt.Printf("IP: %s\n", status.AssignedIP)
fmt.Printf("Connected: %v\n", status.Duration)

// Get traffic stats
stats := client.GetStats()
fmt.Printf("RX: %d bytes\n", stats.RxBytes)
fmt.Printf("TX: %d bytes\n", stats.TxBytes)
fmt.Printf("RX Speed: %d bytes/sec\n", stats.RxSpeed)
fmt.Printf("TX Speed: %d bytes/sec\n", stats.TxSpeed)
```

### Routes

```go
// Get route information
routes := client.GetRoutes()
fmt.Printf("Available: %v\n", routes.AvailableRoutes)
fmt.Printf("Excluded: %v\n", routes.ExcludedRoutes)
fmt.Printf("Active: %v\n", routes.ActiveRoutes)

// Exclude routes from VPN
err := client.SetExcludedRoutes([]string{"192.168.1.0/24", "10.0.0.0/8"})
```

### Events

```go
client.OnEvent(func(event sdk.Event) {
    switch event.Type {
    case sdk.EventConnecting:
        // Show loading indicator

    case sdk.EventConnected:
        // Update UI with connection info
        ip := event.Status.AssignedIP

    case sdk.EventDisconnected:
        // Show disconnected state

    case sdk.EventReconnecting:
        // Show reconnecting indicator

    case sdk.EventError:
        // Show error message
        err := event.Error

    case sdk.EventStatsUpdated:
        // Update traffic display
        rx := event.Stats.RxBytes
        tx := event.Stats.TxBytes

    case sdk.EventRoutesChanged:
        // Refresh route list
    }
})
```

## Event Types

| Event | Description | Available Data |
|-------|-------------|----------------|
| `EventConnecting` | Connection started | `Status` |
| `EventConnected` | Connection established | `Status` |
| `EventDisconnected` | Connection closed | `Status` |
| `EventReconnecting` | Auto-reconnect in progress | `Status` |
| `EventError` | An error occurred | `Error`, `Status` |
| `EventStatsUpdated` | Traffic stats updated | `Stats` |
| `EventRoutesChanged` | Routes were modified | - |

## Connection States

| State | Description |
|-------|-------------|
| `StateDisconnected` | Not connected |
| `StateConnecting` | Connection in progress |
| `StateConnected` | Connected and active |
| `StateFailed` | Connection failed |
| `StateReconnecting` | Reconnecting after disconnect |

## Configuration Options

### SDK Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `ConfigDir` | `string` | `~/.wire-socket` | Config file directory |
| `StatsInterval` | `time.Duration` | `3s` | Stats update interval |
| `Debug` | `bool` | `false` | Enable debug logging |
| `Logger` | `func(string, ...interface{})` | `log.Printf` | Custom logger |

### Connect Config

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `Server` | `string` | - | VPN server address |
| `Username` | `string` | - | Authentication username |
| `Password` | `string` | - | Authentication password |
| `AutoReconnect` | `bool` | `true` | Auto-reconnect on disconnect |
| `ReconnectInterval` | `time.Duration` | `5s` | Initial reconnect interval |
| `MaxReconnectInterval` | `time.Duration` | `60s` | Maximum reconnect interval |
| `ExcludedRoutes` | `[]string` | `nil` | CIDRs to exclude from VPN |

## Examples

See the [examples](./examples) directory for complete working examples:

- **simple-client**: Basic CLI VPN client
- **event-driven-ui**: Reactive UI simulation

## Integration Notes

### Building Custom UIs

The SDK is designed to be UI-agnostic. You can use it with:

- CLI applications (like the simple-client example)
- GUI frameworks (GTK, Qt, Fyne, etc.)
- Web UIs (via HTTP API wrapper)
- Mobile apps (via gomobile)

### Thread Safety

All SDK methods are thread-safe and can be called from any goroutine.

### Platform Support

**Desktop:**
- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64)

**Mobile (via gomobile):**
- iOS (arm64, simulator)
- Android (arm64, arm, x86_64, x86)

Note: Desktop SDK requires root/administrator privileges to create WireGuard interfaces. Mobile SDK uses platform-provided TUN file descriptors.

## Mobile SDK

The `sdk/mobile` package provides gomobile-compatible bindings:

```bash
# Build for Android
gomobile bind -target=android -javapkg=com.wiresocket -o mobile.aar ./mobile

# Build for iOS
gomobile bind -target=ios -o Mobile.xcframework ./mobile
```

### Mobile API

```go
// Create tunnel
tunnel := mobile.NewTunnel()

// Start with platform TUN file descriptor
err := tunnel.StartWithFD(fd, configJSON)

// Get stats
statsJSON := tunnel.GetStats()

// Stop tunnel
tunnel.Stop()
```

See [client/mobile/android](../client/mobile/android) and [client/mobile/ios](../client/mobile/ios) for example implementations.

## Server SDK

The `sdk/server` package provides VPN server functionality:

```go
package main

import (
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/k0ngk0ng/wire-socket/sdk/server"
)

func main() {
    // Create server with config
    cfg := server.DefaultConfig()
    cfg.ListenPort = 51820
    cfg.Address = "10.0.0.1/24"
    cfg.Subnet = "10.0.0.0/24"

    srv, err := server.New(cfg)
    if err != nil {
        log.Fatal(err)
    }

    // Start server
    if err := srv.Start(); err != nil {
        log.Fatal(err)
    }
    defer srv.Stop()

    log.Printf("Server started on port %d", srv.GetListenPort())
    log.Printf("Public key: %s", srv.GetPublicKey())

    // Wait for shutdown signal
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh
}
```

### Server API

```go
// Create and start server
srv, _ := server.New(server.DefaultConfig())
srv.Start()
defer srv.Stop()

// Peer management
srv.AddPeer(publicKey, "10.0.0.2/32")
srv.RemovePeer(publicKey)
peers := srv.GetPeers()

// Get server info
publicKey := srv.GetPublicKey()
port := srv.GetListenPort()
stats, _ := srv.GetPeerStats()
```

## Package Structure

```
sdk/
├── client.go           # VPN client
├── types.go            # Shared types
├── server/             # VPN server
│   ├── server.go       # Server implementation
│   └── ip_allocator.go # IP address allocation
├── wireguard/          # WireGuard backends
│   ├── backend.go      # Backend interface
│   ├── kernel.go       # Kernel mode
│   └── userspace.go    # Userspace mode
└── mobile/             # Mobile bindings
    ├── tunnel.go       # Mobile tunnel
    └── client.go       # Mobile client
```

## License

MIT
