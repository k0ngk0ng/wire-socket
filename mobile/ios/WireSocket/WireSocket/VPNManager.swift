import Foundation
import Combine
import Mobile

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

    private var client: MobileClient?
    private let defaults = UserDefaults.standard

    private let serverKey = "saved_server"
    private let usernameKey = "saved_username"

    init() {
        loadSavedCredentials()
    }

    private func loadSavedCredentials() {
        savedServer = defaults.string(forKey: serverKey) ?? ""
        savedUsername = defaults.string(forKey: usernameKey) ?? ""
    }

    func saveCredentials(server: String, username: String) {
        defaults.set(server, forKey: serverKey)
        defaults.set(username, forKey: usernameKey)
        savedServer = server
        savedUsername = username
    }

    func connect(server: String, username: String, password: String) {
        guard status.state == .disconnected || status.state == .failed else {
            return
        }

        status.state = .connecting
        saveCredentials(server: server, username: username)

        DispatchQueue.global(qos: .userInitiated).async { [weak self] in
            do {
                // Create SDK client
                var error: NSError?
                let newClient = MobileNewClient(&error)
                if let err = error {
                    throw err
                }

                guard let client = newClient else {
                    throw NSError(domain: "VPNManager", code: -1,
                                  userInfo: [NSLocalizedDescriptionKey: "Failed to create client"])
                }

                self?.client = client

                // Set event handler
                client.setEventHandler(EventHandlerImpl(manager: self))

                // Connect
                try client.connect(server, username: username, password: password, autoReconnect: true)

            } catch {
                DispatchQueue.main.async {
                    self?.status.state = .failed
                    self?.status.error = error.localizedDescription
                }
            }
        }
    }

    func disconnect() {
        guard status.state == .connected || status.state == .connecting else {
            return
        }

        status.state = .disconnecting

        DispatchQueue.global(qos: .userInitiated).async { [weak self] in
            do {
                try self?.client?.disconnect()
                try self?.client?.close()
                self?.client = nil
            } catch {
                print("Disconnect error: \(error)")
            }

            DispatchQueue.main.async {
                self?.status = VPNStatus()
            }
        }
    }

    fileprivate func handleEvent(type: String, jsonData: String) {
        guard let data = jsonData.data(using: .utf8),
              let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            return
        }

        DispatchQueue.main.async { [weak self] in
            switch type {
            case "connecting":
                self?.status.state = .connecting

            case "connected":
                if let statusJson = json["status"] as? [String: Any] {
                    self?.status.state = .connected
                    self?.status.assignedIP = statusJson["assignedIP"] as? String ?? ""
                    self?.status.server = statusJson["server"] as? String ?? ""
                    if let timestamp = statusJson["connectedAtUnix"] as? Int64, timestamp > 0 {
                        self?.status.connectedAt = Date(timeIntervalSince1970: TimeInterval(timestamp))
                    }
                }

            case "disconnected":
                self?.status = VPNStatus()

            case "reconnecting":
                self?.status.state = .reconnecting

            case "error":
                self?.status.state = .failed
                self?.status.error = json["error"] as? String ?? "Unknown error"

            case "stats_updated":
                if let stats = json["stats"] as? [String: Any] {
                    self?.status.rxBytes = stats["rxBytes"] as? Int64 ?? 0
                    self?.status.txBytes = stats["txBytes"] as? Int64 ?? 0
                    self?.status.rxSpeed = stats["rxSpeed"] as? Int64 ?? 0
                    self?.status.txSpeed = stats["txSpeed"] as? Int64 ?? 0
                }

            default:
                break
            }
        }
    }
}

// Event handler implementation for Go SDK callback
class EventHandlerImpl: NSObject, MobileEventHandlerProtocol {
    private weak var manager: VPNManager?

    init(manager: VPNManager?) {
        self.manager = manager
    }

    func onEvent(_ eventType: String?, jsonData: String?) {
        guard let type = eventType, let data = jsonData else { return }
        manager?.handleEvent(type: type, jsonData: data)
    }
}
