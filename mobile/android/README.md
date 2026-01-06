# WireSocket Android App

Android VPN client for WireSocket using the Go SDK.

## Prerequisites

- Android Studio Arctic Fox (2021.3.1) or later
- Android SDK 34
- Android NDK (for building Go SDK)
- Go 1.21+
- gomobile (`go install golang.org/x/mobile/cmd/gomobile@latest`)

## Project Structure

```
android/
├── app/
│   ├── libs/                 # Go mobile SDK (.aar)
│   ├── src/main/
│   │   ├── java/com/wiresocket/app/
│   │   │   ├── data/         # Data models and repositories
│   │   │   ├── service/      # VpnService implementation
│   │   │   └── ui/           # Jetpack Compose UI
│   │   ├── res/              # Android resources
│   │   └── AndroidManifest.xml
│   └── build.gradle.kts
├── build.gradle.kts
├── settings.gradle.kts
└── build.sh                  # Build script
```

## Building

### Option 1: Using build script

```bash
./build.sh
```

This will:
1. Build the Go SDK as an `.aar` file
2. Build the Android app

### Option 2: Manual build

1. Build the Go SDK:
```bash
cd ../../sdk
gomobile bind -target=android -androidapi=24 -javapkg=com.wiresocket -o ../mobile/android/app/libs/mobile.aar ./mobile
```

2. Open the project in Android Studio and build.

## Architecture

```
┌─────────────────────────────────────┐
│         Jetpack Compose UI          │
├─────────────────────────────────────┤
│         VpnStateHolder              │
│    (StateFlow for UI updates)       │
├─────────────────────────────────────┤
│       WireSocketVpnService          │
│    (Android VpnService + SDK)       │
├─────────────────────────────────────┤
│          Go Mobile SDK              │
│   (Auth, Tunnel, WireGuard)         │
└─────────────────────────────────────┘
```

## Key Components

### WireSocketVpnService
- Extends Android `VpnService`
- Implements `mobile.EventHandler` to receive SDK events
- Manages foreground notification
- Handles connect/disconnect commands

### VpnStateHolder
- Singleton StateFlow holder
- Shares VPN state between Service and UI
- Updates triggered by SDK events

### MainActivity
- Requests VPN permission
- Displays connection form and status
- Uses Jetpack Compose Material 3

## Current Limitations

The SDK currently uses userspace WireGuard (netstack), which operates in memory without creating a real TUN interface. For full VPN functionality:

1. **Planned**: Modify SDK to expose WireGuard config
2. **Planned**: Create TUN interface via `VpnService.Builder`
3. **Planned**: Route packets between TUN and SDK tunnel

## Permissions

- `INTERNET` - Network access
- `BIND_VPN_SERVICE` - VPN service
- `FOREGROUND_SERVICE` - Keep service running
- `POST_NOTIFICATIONS` - Status notifications
