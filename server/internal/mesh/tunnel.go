package mesh

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// TunnelClient manages a WebSocket tunnel connection to another Mesh node.
// It provides a local UDP endpoint for WireGuard to connect to.
type TunnelClient struct {
	tunnelURL      string
	localKey       string
	remoteKey      string
	localAddr      string
	listener       *net.UDPConn
	wsConn         *websocket.Conn
	mu             sync.Mutex
	running        bool
	stopCh         chan struct{}
	wg             sync.WaitGroup
	lastRemoteAddr *net.UDPAddr
	reconnectDelay time.Duration
}

// NewTunnelClient creates a new tunnel client
func NewTunnelClient(tunnelURL, localPrivateKey, remotePublicKey string) (*TunnelClient, error) {
	// Allocate a local UDP port for WireGuard to connect to
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	listener, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to create UDP listener: %w", err)
	}

	return &TunnelClient{
		tunnelURL:      tunnelURL,
		localKey:       localPrivateKey,
		remoteKey:      remotePublicKey,
		localAddr:      listener.LocalAddr().String(),
		listener:       listener,
		stopCh:         make(chan struct{}),
		reconnectDelay: 5 * time.Second,
	}, nil
}

// Start begins the tunnel operation
func (t *TunnelClient) Start() error {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return nil
	}
	t.running = true
	t.mu.Unlock()

	log.Printf("[mesh-tunnel] Starting tunnel to %s (local: %s)", t.tunnelURL, t.localAddr)

	// Start the tunnel goroutines
	t.wg.Add(2)
	go t.connectLoop()
	go t.readLocalLoop()

	return nil
}

// connectLoop maintains the WebSocket connection with reconnection
func (t *TunnelClient) connectLoop() {
	defer t.wg.Done()

	for {
		select {
		case <-t.stopCh:
			return
		default:
		}

		if err := t.connect(); err != nil {
			log.Printf("[mesh-tunnel] Connection to %s failed: %v", t.tunnelURL, err)
			select {
			case <-t.stopCh:
				return
			case <-time.After(t.reconnectDelay):
				continue
			}
		}

		// Read from WebSocket and forward to local UDP
		t.readWebSocketLoop()

		// Connection lost, try to reconnect
		log.Printf("[mesh-tunnel] Connection to %s lost, reconnecting...", t.tunnelURL)
	}
}

// connect establishes WebSocket connection to the tunnel server
func (t *TunnelClient) connect() error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	header := http.Header{}
	// Add any authentication headers if needed

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, _, err := dialer.DialContext(ctx, t.tunnelURL, header)
	if err != nil {
		return fmt.Errorf("failed to connect to tunnel: %w", err)
	}

	t.mu.Lock()
	t.wsConn = conn
	t.mu.Unlock()

	log.Printf("[mesh-tunnel] Connected to %s", t.tunnelURL)

	// Start ping/pong keepalive
	conn.SetPongHandler(func(appData string) error {
		return nil
	})

	// Send periodic pings
	go t.pingLoop(conn)

	return nil
}

// pingLoop sends periodic ping messages
func (t *TunnelClient) pingLoop(conn *websocket.Conn) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopCh:
			return
		case <-ticker.C:
			t.mu.Lock()
			if t.wsConn != conn {
				t.mu.Unlock()
				return
			}
			t.mu.Unlock()

			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
				return
			}
		}
	}
}

// readWebSocketLoop reads from WebSocket and forwards to local UDP
func (t *TunnelClient) readWebSocketLoop() {
	t.mu.Lock()
	conn := t.wsConn
	t.mu.Unlock()

	if conn == nil {
		return
	}

	for {
		select {
		case <-t.stopCh:
			return
		default:
		}

		// Read message from WebSocket
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		// Forward to local UDP (to WireGuard)
		t.mu.Lock()
		remoteAddr := t.lastRemoteAddr
		t.mu.Unlock()

		if remoteAddr != nil {
			if _, err := t.listener.WriteToUDP(data, remoteAddr); err != nil {
				log.Printf("[mesh-tunnel] Failed to write to UDP: %v", err)
			}
		}
	}
}

// readLocalLoop reads from local UDP and forwards to WebSocket
func (t *TunnelClient) readLocalLoop() {
	defer t.wg.Done()

	buf := make([]byte, 65535)

	for {
		select {
		case <-t.stopCh:
			return
		default:
		}

		// Set read deadline
		t.listener.SetReadDeadline(time.Now().Add(1 * time.Second))

		// Read from local UDP (WireGuard packets)
		n, remoteAddr, err := t.listener.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-t.stopCh:
				return
			default:
				log.Printf("[mesh-tunnel] UDP read error: %v", err)
				continue
			}
		}

		// Remember the remote address for sending responses
		t.mu.Lock()
		t.lastRemoteAddr = remoteAddr
		wsConn := t.wsConn
		t.mu.Unlock()

		// Forward to WebSocket
		if wsConn != nil {
			if err := wsConn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				log.Printf("[mesh-tunnel] WebSocket write error: %v", err)
			}
		}
	}
}

// Close closes the tunnel
func (t *TunnelClient) Close() error {
	t.mu.Lock()
	if !t.running {
		t.mu.Unlock()
		return nil
	}
	t.running = false
	t.mu.Unlock()

	close(t.stopCh)

	// Close WebSocket connection
	t.mu.Lock()
	if t.wsConn != nil {
		t.wsConn.Close()
		t.wsConn = nil
	}
	t.mu.Unlock()

	// Close UDP listener
	if t.listener != nil {
		t.listener.Close()
	}

	// Wait for goroutines
	t.wg.Wait()

	log.Printf("[mesh-tunnel] Closed tunnel to %s", t.tunnelURL)
	return nil
}

// GetLocalEndpoint returns the local UDP endpoint for WireGuard
func (t *TunnelClient) GetLocalEndpoint() string {
	return t.localAddr
}

// IsConnected returns whether the tunnel is connected
func (t *TunnelClient) IsConnected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running && t.wsConn != nil
}
