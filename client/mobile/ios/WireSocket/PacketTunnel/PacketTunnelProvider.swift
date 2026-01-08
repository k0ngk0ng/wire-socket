import NetworkExtension
import Mobile

class PacketTunnelProvider: NEPacketTunnelProvider {

    private var tunnel: MobileTunnel?
    private var pendingCompletion: ((Error?) -> Void)?

    override func startTunnel(options: [String : NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        NSLog("PacketTunnelProvider: startTunnel called")

        guard let options = options,
              let server = options["server"] as? String,
              let username = options["username"] as? String,
              let password = options["password"] as? String else {
            completionHandler(NSError(domain: "PacketTunnelProvider", code: -1,
                                       userInfo: [NSLocalizedDescriptionKey: "Missing connection parameters"]))
            return
        }

        pendingCompletion = completionHandler

        // Authenticate and get WireGuard config
        authenticateAndConnect(server: server, username: username, password: password)
    }

    override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        NSLog("PacketTunnelProvider: stopTunnel called, reason: \(reason)")

        tunnel?.stop()
        tunnel = nil

        completionHandler()
    }

    override func handleAppMessage(_ messageData: Data, completionHandler: ((Data?) -> Void)?) {
        // Handle messages from the main app if needed
        if let message = String(data: messageData, encoding: .utf8) {
            NSLog("PacketTunnelProvider: received app message: \(message)")

            if message == "getStats" {
                if let statsJson = tunnel?.getStats(),
                   let data = statsJson.data(using: .utf8) {
                    completionHandler?(data)
                    return
                }
            }
        }
        completionHandler?(nil)
    }

    private func authenticateAndConnect(server: String, username: String, password: String) {
        DispatchQueue.global(qos: .userInitiated).async { [weak self] in
            do {
                // Authenticate
                let config = try self?.authenticate(server: server, username: username, password: password)
                guard let wgConfig = config else {
                    throw NSError(domain: "PacketTunnelProvider", code: -1,
                                  userInfo: [NSLocalizedDescriptionKey: "Failed to get WireGuard config"])
                }

                // Configure network settings
                try self?.configureNetworkSettings(config: wgConfig)

                // Start WireGuard tunnel
                try self?.startWireGuardTunnel(config: wgConfig)

                NSLog("PacketTunnelProvider: tunnel started successfully")
                self?.pendingCompletion?(nil)
                self?.pendingCompletion = nil

            } catch {
                NSLog("PacketTunnelProvider: error starting tunnel: \(error)")
                self?.pendingCompletion?(error)
                self?.pendingCompletion = nil
            }
        }
    }

    private func authenticate(server: String, username: String, password: String) throws -> WireGuardConfig {
        let baseURL = normalizeServerURL(server)

        // Login
        let loginURL = URL(string: "\(baseURL)/api/auth/login")!
        var loginRequest = URLRequest(url: loginURL)
        loginRequest.httpMethod = "POST"
        loginRequest.setValue("application/json", forHTTPHeaderField: "Content-Type")

        let loginData = try JSONSerialization.data(withJSONObject: [
            "username": username,
            "password": password
        ])
        loginRequest.httpBody = loginData

        var loginResponse: [String: Any]?
        let loginSemaphore = DispatchSemaphore(value: 0)
        var loginError: Error?

        let loginTask = URLSession.shared.dataTask(with: loginRequest) { data, response, error in
            defer { loginSemaphore.signal() }
            if let error = error {
                loginError = error
                return
            }
            if let data = data {
                loginResponse = try? JSONSerialization.jsonObject(with: data) as? [String: Any]
            }
        }
        loginTask.resume()
        loginSemaphore.wait()

        if let error = loginError {
            throw error
        }

        guard let token = loginResponse?["token"] as? String else {
            throw NSError(domain: "PacketTunnelProvider", code: -1,
                          userInfo: [NSLocalizedDescriptionKey: "No token in login response"])
        }

        // Get config
        let configURL = URL(string: "\(baseURL)/api/config")!
        var configRequest = URLRequest(url: configURL)
        configRequest.httpMethod = "GET"
        configRequest.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")

        var configResponse: [String: Any]?
        let configSemaphore = DispatchSemaphore(value: 0)
        var configError: Error?

        let configTask = URLSession.shared.dataTask(with: configRequest) { data, response, error in
            defer { configSemaphore.signal() }
            if let error = error {
                configError = error
                return
            }
            if let data = data {
                configResponse = try? JSONSerialization.jsonObject(with: data) as? [String: Any]
            }
        }
        configTask.resume()
        configSemaphore.wait()

        if let error = configError {
            throw error
        }

        guard let configJson = configResponse,
              let wgConfigJson = configJson["config"] as? [String: Any],
              let peerJson = wgConfigJson["peer"] as? [String: Any] else {
            throw NSError(domain: "PacketTunnelProvider", code: -1,
                          userInfo: [NSLocalizedDescriptionKey: "Invalid config response"])
        }

        let allowedIPsString = peerJson["allowed_ips"] as? String ?? ""
        let allowedIPs = allowedIPsString.split(separator: ",").map { String($0).trimmingCharacters(in: .whitespaces) }

        let tunnelURL = configJson["tunnel_url"] as? String ?? server
        let peerEndpoint = buildPeerEndpoint(tunnelURL: tunnelURL, fallback: server)

        return WireGuardConfig(
            privateKey: wgConfigJson["private_key"] as? String ?? "",
            address: wgConfigJson["address"] as? String ?? "",
            dns: wgConfigJson["dns"] as? String ?? "",
            peerPublicKey: peerJson["public_key"] as? String ?? "",
            peerEndpoint: peerEndpoint,
            allowedIPs: allowedIPs
        )
    }

    private func configureNetworkSettings(config: WireGuardConfig) throws {
        let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: config.peerEndpoint.split(separator: ":").first.map(String.init) ?? "")

        // Parse address
        let addressParts = config.address.split(separator: "/")
        let address = String(addressParts[0])
        let prefixLength = addressParts.count > 1 ? Int(addressParts[1]) ?? 32 : 32

        // IPv4 settings
        let ipv4Settings = NEIPv4Settings(addresses: [address], subnetMasks: [prefixLengthToMask(prefixLength)])

        // Add routes
        var includedRoutes: [NEIPv4Route] = []
        for allowedIP in config.allowedIPs {
            let parts = allowedIP.split(separator: "/")
            if parts.count >= 1 {
                let routeAddress = String(parts[0])
                let routePrefix = parts.count > 1 ? Int(parts[1]) ?? 32 : 32
                let route = NEIPv4Route(destinationAddress: routeAddress, subnetMask: prefixLengthToMask(routePrefix))
                includedRoutes.append(route)
            }
        }
        ipv4Settings.includedRoutes = includedRoutes
        settings.ipv4Settings = ipv4Settings

        // DNS settings
        if !config.dns.isEmpty {
            let dnsServers = config.dns.split(separator: ",").map { String($0).trimmingCharacters(in: .whitespaces) }
            settings.dnsSettings = NEDNSSettings(servers: dnsServers)
        }

        // MTU
        settings.mtu = NSNumber(value: 1420)

        let semaphore = DispatchSemaphore(value: 0)
        var settingsError: Error?

        setTunnelNetworkSettings(settings) { error in
            settingsError = error
            semaphore.signal()
        }

        semaphore.wait()

        if let error = settingsError {
            throw error
        }
    }

    private func startWireGuardTunnel(config: WireGuardConfig) throws {
        // Get the tunnel file descriptor
        guard let tunnelFileDescriptor = (value(forKey: "packetFlow") as? NEPacketTunnelFlow)?.value(forKey: "fileDescriptor") as? Int32 else {
            // Alternative: use packetFlow directly with readPackets/writePackets
            // For now, we'll use the Tunnel API which handles this internally
            throw NSError(domain: "PacketTunnelProvider", code: -1,
                          userInfo: [NSLocalizedDescriptionKey: "Could not get tunnel file descriptor"])
        }

        // Create tunnel config JSON
        let tunnelConfigDict: [String: Any] = [
            "privateKey": config.privateKey,
            "address": config.address,
            "dns": config.dns,
            "peerPublicKey": config.peerPublicKey,
            "peerEndpoint": config.peerEndpoint,
            "allowedIPs": config.allowedIPs,
            "mtu": 1420
        ]

        let configData = try JSONSerialization.data(withJSONObject: tunnelConfigDict)
        let configJSON = String(data: configData, encoding: .utf8) ?? ""

        // Start WireGuard tunnel
        tunnel = MobileNewTunnel()
        try tunnel?.startWithFD(Int(tunnelFileDescriptor), configJSON: configJSON)
    }

    private func normalizeServerURL(_ server: String) -> String {
        var url = server.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        if !url.hasPrefix("http://") && !url.hasPrefix("https://") {
            if url.contains(":") && url.split(separator: ":").last != "443" {
                url = "http://\(url)"
            } else {
                url = "https://\(url)"
            }
        }
        return url
    }

    private func buildPeerEndpoint(tunnelURL: String, fallback: String) -> String {
        let urlString = normalizeServerURL(tunnelURL.isEmpty ? fallback : tunnelURL)
        if let url = URL(string: urlString), let host = url.host {
            return "\(host):51820"
        }
        return "127.0.0.1:51820"
    }

    private func prefixLengthToMask(_ length: Int) -> String {
        var mask: UInt32 = 0
        for i in 0..<length {
            mask |= (1 << (31 - i))
        }
        return "\((mask >> 24) & 0xFF).\((mask >> 16) & 0xFF).\((mask >> 8) & 0xFF).\(mask & 0xFF)"
    }
}

struct WireGuardConfig {
    let privateKey: String
    let address: String
    let dns: String
    let peerPublicKey: String
    let peerEndpoint: String
    let allowedIPs: [String]
}
