import Foundation
import Combine
import NetworkExtension

// Note: WireSocketSDK is used by the PacketTunnel extension, not the main app

enum ConnectionState: String {
    case disconnected
    case connecting
    case connected
    case disconnecting
    case reconnecting
    case failed
}

struct VPNStatus {
    var state: ConnectionState = .disconnected
    var server: String = ""
    var assignedIP: String = ""
    var connectedAt: Date?
    var error: String = ""
    var rxBytes: Int64 = 0
    var txBytes: Int64 = 0
    var rxSpeed: Int64 = 0
    var txSpeed: Int64 = 0
}

class VPNManager: ObservableObject {
    @Published var status = VPNStatus()
    @Published var savedServer: String = ""
    @Published var savedUsername: String = ""
    @Published var savedPassword: String = ""

    private var vpnManager: NETunnelProviderManager?
    private var statusObserver: NSObjectProtocol?
    private var statsTimer: Timer?

    private let defaults = UserDefaults.standard
    private let serverKey = "saved_server"
    private let usernameKey = "saved_username"
    private let passwordKeychainAccount = "vpn_password"

    // Bundle identifier for the Network Extension
    private let tunnelBundleId = "com.wiresocket.WireSocket.PacketTunnel"

    init() {
        loadSavedCredentials()
        loadVPNConfiguration()
        setupStatusObserver()
    }

    deinit {
        if let observer = statusObserver {
            NotificationCenter.default.removeObserver(observer)
        }
        statsTimer?.invalidate()
    }

    private func loadSavedCredentials() {
        savedServer = defaults.string(forKey: serverKey) ?? ""
        savedUsername = defaults.string(forKey: usernameKey) ?? ""
        savedPassword = KeychainHelper.shared.get(for: passwordKeychainAccount) ?? ""
    }

    func saveCredentials(server: String, username: String, password: String) {
        defaults.set(server, forKey: serverKey)
        defaults.set(username, forKey: usernameKey)
        KeychainHelper.shared.save(password: password, for: passwordKeychainAccount)
        savedServer = server
        savedUsername = username
        savedPassword = password
    }

    private func loadVPNConfiguration() {
        NETunnelProviderManager.loadAllFromPreferences { [weak self] managers, error in
            if let error = error {
                print("Failed to load VPN configuration: \(error)")
                return
            }

            // Find our VPN manager or create a new one
            self?.vpnManager = managers?.first { manager in
                (manager.protocolConfiguration as? NETunnelProviderProtocol)?.providerBundleIdentifier == self?.tunnelBundleId
            }

            // Update current status
            if let manager = self?.vpnManager {
                self?.updateStatus(from: manager.connection.status)
            }
        }
    }

    private func setupStatusObserver() {
        statusObserver = NotificationCenter.default.addObserver(
            forName: .NEVPNStatusDidChange,
            object: nil,
            queue: .main
        ) { [weak self] notification in
            if let connection = notification.object as? NEVPNConnection {
                self?.updateStatus(from: connection.status)
            }
        }
    }

    private func updateStatus(from vpnStatus: NEVPNStatus) {
        switch vpnStatus {
        case .invalid:
            status.state = .disconnected
        case .disconnected:
            status.state = .disconnected
            stopStatsTimer()
        case .connecting:
            status.state = .connecting
        case .connected:
            status.state = .connected
            status.connectedAt = Date()
            startStatsTimer()
        case .reasserting:
            status.state = .reconnecting
        case .disconnecting:
            status.state = .disconnecting
        @unknown default:
            status.state = .disconnected
        }
    }

    func connect(server: String, username: String, password: String) {
        guard status.state == .disconnected || status.state == .failed else {
            return
        }

        status.state = .connecting
        saveCredentials(server: server, username: username, password: password)
        status.server = server

        // Create or update VPN configuration
        createOrUpdateVPNConfiguration(server: server, username: username, password: password)
    }

    private func createOrUpdateVPNConfiguration(server: String, username: String, password: String) {
        let manager = vpnManager ?? NETunnelProviderManager()

        let protocolConfig = NETunnelProviderProtocol()
        protocolConfig.providerBundleIdentifier = tunnelBundleId
        protocolConfig.serverAddress = server

        // Store credentials in protocol configuration
        // Note: In production, use Keychain for passwords
        protocolConfig.providerConfiguration = [
            "server": server,
            "username": username,
            "password": password
        ]

        manager.protocolConfiguration = protocolConfig
        manager.localizedDescription = "WireSocket VPN"
        manager.isEnabled = true

        manager.saveToPreferences { [weak self] error in
            if let error = error {
                DispatchQueue.main.async {
                    self?.status.state = .failed
                    self?.status.error = error.localizedDescription
                }
                return
            }

            // Reload and connect
            manager.loadFromPreferences { [weak self] error in
                if let error = error {
                    DispatchQueue.main.async {
                        self?.status.state = .failed
                        self?.status.error = error.localizedDescription
                    }
                    return
                }

                self?.vpnManager = manager
                self?.startVPNConnection(server: server, username: username, password: password)
            }
        }
    }

    private func startVPNConnection(server: String, username: String, password: String) {
        guard let manager = vpnManager else {
            status.state = .failed
            status.error = "VPN manager not configured"
            return
        }

        do {
            // Pass options to the Network Extension
            let options: [String: NSObject] = [
                "server": server as NSObject,
                "username": username as NSObject,
                "password": password as NSObject
            ]

            try manager.connection.startVPNTunnel(options: options)
        } catch {
            DispatchQueue.main.async { [weak self] in
                self?.status.state = .failed
                self?.status.error = error.localizedDescription
            }
        }
    }

    func disconnect() {
        guard status.state == .connected || status.state == .connecting else {
            return
        }

        status.state = .disconnecting
        vpnManager?.connection.stopVPNTunnel()
    }

    // MARK: - Stats

    private func startStatsTimer() {
        statsTimer = Timer.scheduledTimer(withTimeInterval: 3.0, repeats: true) { [weak self] _ in
            self?.fetchStats()
        }
    }

    private func stopStatsTimer() {
        statsTimer?.invalidate()
        statsTimer = nil
    }

    private func fetchStats() {
        guard let session = vpnManager?.connection as? NETunnelProviderSession else {
            return
        }

        do {
            try session.sendProviderMessage("getStats".data(using: .utf8)!) { [weak self] response in
                guard let data = response,
                      let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else {
                    return
                }

                DispatchQueue.main.async {
                    self?.status.rxBytes = json["rxBytes"] as? Int64 ?? 0
                    self?.status.txBytes = json["txBytes"] as? Int64 ?? 0
                }
            }
        } catch {
            print("Failed to fetch stats: \(error)")
        }
    }
}
