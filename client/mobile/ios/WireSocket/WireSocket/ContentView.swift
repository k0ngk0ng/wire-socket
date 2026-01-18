import SwiftUI

struct ContentView: View {
    @EnvironmentObject var vpnManager: VPNManager

    @State private var server: String = ""
    @State private var username: String = ""
    @State private var password: String = ""

    var body: some View {
        NavigationStack {
            VStack(spacing: 20) {
                // Status Card
                StatusCard(status: vpnManager.status)
                    .padding(.horizontal)

                Spacer().frame(height: 20)

                // Connection Form
                if vpnManager.status.state == .disconnected || vpnManager.status.state == .failed {
                    VStack(spacing: 16) {
                        TextField("Server", text: $server)
                            .textFieldStyle(.roundedBorder)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()

                        TextField("Username", text: $username)
                            .textFieldStyle(.roundedBorder)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()

                        SecureField("Password", text: $password)
                            .textFieldStyle(.roundedBorder)
                    }
                    .padding(.horizontal)
                }

                // Error Message
                if vpnManager.status.state == .failed && !vpnManager.status.error.isEmpty {
                    Text(vpnManager.status.error)
                        .foregroundColor(.white)
                        .padding()
                        .background(Color.red.opacity(0.8))
                        .cornerRadius(8)
                        .padding(.horizontal)
                }

                Spacer()

                // Connect/Disconnect Button
                Button(action: {
                    if vpnManager.status.state == .connected ||
                       vpnManager.status.state == .connecting ||
                       vpnManager.status.state == .reconnecting {
                        vpnManager.disconnect()
                    } else {
                        vpnManager.connect(server: server, username: username, password: password)
                    }
                }) {
                    HStack {
                        if vpnManager.status.state == .connecting ||
                           vpnManager.status.state == .reconnecting {
                            ProgressView()
                                .progressViewStyle(CircularProgressViewStyle(tint: .white))
                                .padding(.trailing, 8)
                        }
                        Text(buttonTitle)
                            .fontWeight(.semibold)
                    }
                    .frame(maxWidth: .infinity)
                    .padding()
                    .background(buttonColor)
                    .foregroundColor(.white)
                    .cornerRadius(12)
                }
                .disabled(!canTapButton)
                .padding(.horizontal)
                .padding(.bottom, 30)
            }
            .navigationTitle("WireSocket")
            .onAppear {
                server = vpnManager.savedServer
                username = vpnManager.savedUsername
                password = vpnManager.savedPassword
            }
        }
    }

    private var buttonTitle: String {
        switch vpnManager.status.state {
        case .disconnected, .failed:
            return "Connect"
        case .connecting:
            return "Connecting..."
        case .connected:
            return "Disconnect"
        case .disconnecting:
            return "Disconnecting..."
        case .reconnecting:
            return "Reconnecting..."
        }
    }

    private var buttonColor: Color {
        switch vpnManager.status.state {
        case .connected:
            return .red
        case .connecting, .reconnecting, .disconnecting:
            return .gray
        default:
            return .blue
        }
    }

    private var canTapButton: Bool {
        switch vpnManager.status.state {
        case .disconnected, .failed:
            return !server.isEmpty && !username.isEmpty && !password.isEmpty
        case .connected:
            return true
        default:
            return false
        }
    }
}

struct StatusCard: View {
    let status: VPNStatus

    var body: some View {
        VStack(spacing: 12) {
            Text(statusTitle)
                .font(.title)
                .fontWeight(.bold)

            if status.state == .connected {
                Text(status.assignedIP)
                    .font(.headline)
                    .foregroundColor(.secondary)

                HStack(spacing: 40) {
                    VStack {
                        Text("↓ \(formatBytes(status.rxBytes))")
                            .font(.subheadline)
                        Text("\(formatBytes(status.rxSpeed))/s")
                            .font(.caption)
                            .foregroundColor(.secondary)
                    }
                    VStack {
                        Text("↑ \(formatBytes(status.txBytes))")
                            .font(.subheadline)
                        Text("\(formatBytes(status.txSpeed))/s")
                            .font(.caption)
                            .foregroundColor(.secondary)
                    }
                }
            }
        }
        .frame(maxWidth: .infinity)
        .padding()
        .background(statusColor.opacity(0.15))
        .cornerRadius(16)
    }

    private var statusTitle: String {
        switch status.state {
        case .disconnected: return "Disconnected"
        case .connecting: return "Connecting"
        case .connected: return "Connected"
        case .disconnecting: return "Disconnecting"
        case .reconnecting: return "Reconnecting"
        case .failed: return "Failed"
        }
    }

    private var statusColor: Color {
        switch status.state {
        case .connected: return .green
        case .connecting, .reconnecting: return .orange
        case .failed: return .red
        default: return .gray
        }
    }

    private func formatBytes(_ bytes: Int64) -> String {
        if bytes < 1024 {
            return "\(bytes) B"
        } else if bytes < 1024 * 1024 {
            return "\(bytes / 1024) KB"
        } else if bytes < 1024 * 1024 * 1024 {
            return "\(bytes / (1024 * 1024)) MB"
        } else {
            return "\(bytes / (1024 * 1024 * 1024)) GB"
        }
    }
}

#Preview {
    ContentView()
        .environmentObject(VPNManager())
}
