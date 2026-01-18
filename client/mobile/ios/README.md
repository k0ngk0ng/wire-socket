# WireSocket iOS App

iOS VPN client for WireSocket using the Go SDK and SwiftUI.

## Prerequisites

- Xcode 15.0 or later
- iOS 16.0+ deployment target
- Go 1.21+
- gomobile (`go install golang.org/x/mobile/cmd/gomobile@latest`)
- **Paid Apple Developer Account ($99/year)** - Required for Network Extension (VPN) entitlement

## Project Structure

```
ios/
├── WireSocket/
│   ├── WireSocket.xcodeproj/
│   ├── WireSocket/
│   │   ├── WireSocketApp.swift    # App entry point
│   │   ├── ContentView.swift      # Main UI
│   │   ├── VPNManager.swift       # VPN state management
│   │   ├── Assets.xcassets/       # App icons and colors
│   │   └── Info.plist
│   └── Frameworks/
│       └── WireSocketSDK.xcframework  # Go SDK (generated)
├── build.sh                       # Build script
└── README.md
```

## Building

### Option 1: Using build script

```bash
./build.sh
```

This will:
1. Build the Go SDK as an `.xcframework`
2. Build the iOS app for simulator

### Option 2: Manual build

1. Build the Go SDK:
```bash
cd ../../../sdk/mobile
gomobile bind -v -target=ios -o WireSocketSDK.xcframework .
cp -r WireSocketSDK.xcframework ../../client/mobile/ios/WireSocket/Frameworks/
```

2. Open in Xcode:
```bash
open WireSocket/WireSocket.xcodeproj
```

3. Select a simulator or device and build.

## Architecture

```
┌─────────────────────────────────────┐
│           SwiftUI Views             │
│    (ContentView, StatusCard)        │
├─────────────────────────────────────┤
│           VPNManager                │
│   (ObservableObject + Combine)      │
├─────────────────────────────────────┤
│      WireSocketSDK.xcframework      │
│   (Go SDK via gomobile bind)        │
└─────────────────────────────────────┘
```

## Key Components

### VPNManager
- `ObservableObject` for SwiftUI state management
- Implements `MobileEventHandlerProtocol` for SDK callbacks
- Manages connection lifecycle
- Persists credentials in UserDefaults

### ContentView
- Main UI with connection form
- Status card showing connection state and stats
- Connect/Disconnect button

## Current Status

**Update (v0.8.x):** The iOS app now includes a Network Extension (PacketTunnelProvider) for full system-wide VPN functionality:

1. Main app uses `NETunnelProviderManager` to configure and control VPN
2. PacketTunnel extension handles actual VPN traffic
3. SDK's `Tunnel` class accepts TUN file descriptor from the extension
4. All traffic is routed through WireGuard tunnel

### Architecture with Network Extension

```
┌─────────────────────────────────────┐
│           SwiftUI Views             │
│    (ContentView, StatusCard)        │
├─────────────────────────────────────┤
│           VPNManager                │
│   (NETunnelProviderManager)         │
├─────────────────────────────────────┤
│      PacketTunnelProvider           │
│   (NEPacketTunnelProvider)          │
├─────────────────────────────────────┤
│      WireSocketSDK.xcframework      │
│   (Go SDK via gomobile bind)        │
└─────────────────────────────────────┘
```

## Development Notes

- The Go SDK is built as an XCFramework for both simulator and device
- Event handling uses the `MobileEventHandlerProtocol` interface
- JSON is used for data exchange between Swift and Go

## Signing

For device deployment:
1. Open project in Xcode
2. Select your development team in Signing & Capabilities (requires paid Apple Developer account)
3. Update bundle identifier if needed
4. Select both **WireSocket** and **PacketTunnel** targets and set the same Team

**Note:** Free Apple Developer accounts cannot use Network Extension entitlement. VPN apps require a paid account ($99/year).
