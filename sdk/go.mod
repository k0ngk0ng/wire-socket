module github.com/k0ngk0ng/wire-socket/sdk

go 1.24.0

require (
	github.com/gorilla/websocket v1.5.1
	github.com/k0ngk0ng/wire-socket/pkg/wireguard v0.0.0
)

require (
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/josharian/native v1.1.0 // indirect
	github.com/mdlayher/genetlink v1.3.2 // indirect
	github.com/mdlayher/netlink v1.7.2 // indirect
	github.com/mdlayher/socket v0.4.1 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/mobile v0.0.0-20251209145715-2553ed8ce294 // indirect
	golang.org/x/mod v0.31.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/tools v0.40.0 // indirect
	golang.zx2c4.com/wintun v0.0.0-20230126152724-0fa3db229ce2 // indirect
	golang.zx2c4.com/wireguard v0.0.0-20231211153847-12269c276173 // indirect
	golang.zx2c4.com/wireguard/wgctrl v0.0.0-20230429144221-925a1e7659e6 // indirect
)

replace (
	github.com/k0ngk0ng/wire-socket/pkg/wireguard => ../pkg/wireguard
	github.com/k0ngk0ng/wire-socket/pkg/wstunnel => ../pkg/wstunnel
)
