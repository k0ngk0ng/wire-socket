// Package wstunnel provides WebSocket-UDP tunnel client functionality.
// This replaces the external wstunnel binary dependency.
package wstunnel

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// DefaultBufferSize is the default buffer size for UDP packets
	DefaultBufferSize = 65535

	// DefaultTimeout is the default connection timeout
	DefaultTimeout = 30 * time.Second

	// PingInterval is the interval between WebSocket ping messages
	PingInterval = 30 * time.Second

	// PongTimeout is the timeout waiting for pong response
	PongTimeout = 10 * time.Second

	// ReconnectInterval is the base interval for reconnection attempts
	ReconnectInterval = 5 * time.Second

	// MaxReconnectInterval is the maximum interval for reconnection attempts
	MaxReconnectInterval = 60 * time.Second
)

// Client handles UDP listening and forwards to WebSocket
type Client struct {
	localAddr  string          // Local UDP listen address (e.g., "127.0.0.1:51820")
	serverURL  string          // WebSocket server URL (e.g., "wss://server:443")
	conn       *websocket.Conn
	udpConn    *net.UDPConn
	mu         sync.Mutex
	running    bool
	stopChan   chan struct{}
	insecure   bool // Skip TLS verification
	actualPort int  // Actual port after binding (useful when using port 0)
	lastPong   time.Time
	connMu     sync.RWMutex // Protects WebSocket connection for ping/pong
}

// Config holds client configuration
type Config struct {
	LocalAddr string // Local UDP listen address
	ServerURL string // WebSocket server URL
	Insecure  bool   // Skip TLS verification (for self-signed certs)
}

// NewClient creates a new WebSocket tunnel client
func NewClient(cfg Config) *Client {
	return &Client{
		localAddr: cfg.LocalAddr,
		serverURL: cfg.ServerURL,
		insecure:  cfg.Insecure,
		stopChan:  make(chan struct{}),
	}
}

// Start starts the WebSocket tunnel client
func (c *Client) Start() error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("client already running")
	}
	c.running = true
	c.stopChan = make(chan struct{})
	c.mu.Unlock()

	// Listen on local UDP
	udpAddr, err := net.ResolveUDPAddr("udp", c.localAddr)
	if err != nil {
		return fmt.Errorf("failed to resolve local UDP address: %w", err)
	}

	c.udpConn, err = net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP %s: %w", c.localAddr, err)
	}

	// Store the actual port (useful when binding to port 0)
	c.actualPort = c.udpConn.LocalAddr().(*net.UDPAddr).Port

	// Connect to WebSocket server
	if err := c.connectWebSocket(); err != nil {
		c.udpConn.Close()
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
		return err
	}

	log.Printf("Tunnel client started: UDP %s <-> WS %s", c.localAddr, c.serverURL)

	// Track client addresses for responses
	clientMap := make(map[string]*net.UDPAddr)
	var clientMu sync.Mutex

	// Start forwarding goroutines
	go c.udpToWS(clientMap, &clientMu)
	go c.wsToUDP(clientMap, &clientMu)
	go c.pingLoop()

	return nil
}

// connectWebSocket establishes WebSocket connection
func (c *Client) connectWebSocket() error {
	dialer := websocket.Dialer{
		HandshakeTimeout: DefaultTimeout,
	}

	if c.insecure {
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	conn, _, err := dialer.Dial(c.serverURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to WebSocket server %s: %w", c.serverURL, err)
	}

	c.connMu.Lock()
	c.conn = conn
	c.lastPong = time.Now()
	c.connMu.Unlock()

	// Set pong handler
	conn.SetPongHandler(func(appData string) error {
		c.connMu.Lock()
		c.lastPong = time.Now()
		c.connMu.Unlock()
		return nil
	})

	return nil
}

// reconnect attempts to reconnect with exponential backoff
func (c *Client) reconnect() bool {
	interval := ReconnectInterval

	for {
		select {
		case <-c.stopChan:
			return false
		default:
		}

		log.Printf("Attempting to reconnect to %s...", c.serverURL)

		if err := c.connectWebSocket(); err != nil {
			log.Printf("Reconnection failed: %v, retrying in %v", err, interval)

			select {
			case <-c.stopChan:
				return false
			case <-time.After(interval):
			}

			// Exponential backoff
			interval = interval * 2
			if interval > MaxReconnectInterval {
				interval = MaxReconnectInterval
			}
			continue
		}

		log.Printf("Reconnected to %s", c.serverURL)
		return true
	}
}

// pingLoop sends periodic ping messages to keep connection alive
func (c *Client) pingLoop() {
	ticker := time.NewTicker(PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.connMu.RLock()
			conn := c.conn
			lastPong := c.lastPong
			c.connMu.RUnlock()

			if conn == nil {
				continue
			}

			// Check if we received pong recently
			if time.Since(lastPong) > PingInterval+PongTimeout {
				log.Printf("No pong received for %v, connection may be dead", time.Since(lastPong))
				c.connMu.Lock()
				if c.conn != nil {
					c.conn.Close()
					c.conn = nil
				}
				c.connMu.Unlock()

				// Try to reconnect
				if !c.reconnect() {
					return
				}
				continue
			}

			// Send ping
			c.connMu.Lock()
			if c.conn != nil {
				err := c.conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(PongTimeout))
				if err != nil {
					log.Printf("Failed to send ping: %v", err)
					c.conn.Close()
					c.conn = nil
					c.connMu.Unlock()

					// Try to reconnect
					if !c.reconnect() {
						return
					}
					continue
				}
			}
			c.connMu.Unlock()
		}
	}
}

// Stop stops the client
func (c *Client) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	c.running = false
	close(c.stopChan)

	c.connMu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.connMu.Unlock()

	if c.udpConn != nil {
		c.udpConn.Close()
	}

	log.Println("Tunnel client stopped")
	return nil
}

// IsRunning returns whether the client is running
func (c *Client) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// LocalPort returns the actual local UDP port the client is listening on.
// This is useful when the client was configured with port 0 (dynamic port).
func (c *Client) LocalPort() int {
	return c.actualPort
}

// udpToWS forwards data from local UDP to WebSocket
func (c *Client) udpToWS(clientMap map[string]*net.UDPAddr, mu *sync.Mutex) {
	buf := make([]byte, DefaultBufferSize)
	for {
		select {
		case <-c.stopChan:
			return
		default:
		}

		c.udpConn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, addr, err := c.udpConn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-c.stopChan:
				return
			default:
				log.Printf("UDP read error: %v", err)
				return
			}
		}

		// Store client address for response routing
		mu.Lock()
		clientMap["last"] = addr
		mu.Unlock()

		// Get current connection with lock
		c.connMu.RLock()
		conn := c.conn
		c.connMu.RUnlock()

		if conn == nil {
			// Connection not ready, wait for reconnect
			time.Sleep(100 * time.Millisecond)
			continue
		}

		err = conn.WriteMessage(websocket.BinaryMessage, buf[:n])
		if err != nil {
			log.Printf("WebSocket write error: %v", err)
			// Don't return, pingLoop will handle reconnection
			time.Sleep(100 * time.Millisecond)
			continue
		}
	}
}

// wsToUDP forwards data from WebSocket to local UDP clients
func (c *Client) wsToUDP(clientMap map[string]*net.UDPAddr, mu *sync.Mutex) {
	for {
		select {
		case <-c.stopChan:
			return
		default:
		}

		// Get current connection with lock
		c.connMu.RLock()
		conn := c.conn
		c.connMu.RUnlock()

		if conn == nil {
			// Connection not ready, wait for reconnect
			time.Sleep(100 * time.Millisecond)
			continue
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			select {
			case <-c.stopChan:
				return
			default:
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket read error: %v", err)
				}
				// Don't return, wait for reconnection
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}

		// Send response to last known client
		mu.Lock()
		clientAddr := clientMap["last"]
		mu.Unlock()

		if clientAddr != nil {
			_, err = c.udpConn.WriteToUDP(data, clientAddr)
			if err != nil {
				log.Printf("UDP write error: %v", err)
			}
		}
	}
}
