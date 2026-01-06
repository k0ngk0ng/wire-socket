# WireSocket iOS App

iOS VPN client for WireSocket using the Go SDK and SwiftUI.

## Prerequisites

- Xcode 15.0 or later
- iOS 16.0+ deployment target
- Go 1.21+
- gomobile (`go install golang.org/x/mobile/cmd/gomobile@latest`)

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
│       └── Mobile.xcframework     # Go SDK (generated)
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
cd ../../sdk
gomobile bind -target=ios -o ../mobile/ios/WireSocket/Frameworks/Mobile.xcframework ./mobile
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
│      Mobile.xcframework             │
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

## Current Limitations

The app currently uses the SDK's userspace WireGuard implementation. For full iOS VPN functionality with system-wide traffic routing, a Network Extension would be needed:

1. **Planned**: Add Packet Tunnel Provider extension
2. **Planned**: Integrate with NEVPNManager
3. **Planned**: Handle extension-app communication

## Development Notes

- The Go SDK is built as an XCFramework for both simulator and device
- Event handling uses the `MobileEventHandlerProtocol` interface
- JSON is used for data exchange between Swift and Go

## Signing

For device deployment:
1. Open project in Xcode
2. Select your development team in Signing & Capabilities
3. Update bundle identifier if needed
