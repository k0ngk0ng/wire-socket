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
)

// Client handles UDP listening and forwards to WebSocket
type Client struct {
	localAddr string          // Local UDP listen address (e.g., "127.0.0.1:51820")
	serverURL string          // WebSocket server URL (e.g., "wss://server:443")
	conn      *websocket.Conn
	udpConn   *net.UDPConn
	mu        sync.Mutex
	running   bool
	stopChan  chan struct{}
	insecure  bool // Skip TLS verification
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

	// Connect to WebSocket server
	dialer := websocket.Dialer{
		HandshakeTimeout: DefaultTimeout,
	}

	if c.insecure {
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	c.conn, _, err = dialer.Dial(c.serverURL, nil)
	if err != nil {
		c.udpConn.Close()
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
		return fmt.Errorf("failed to connect to WebSocket server %s: %w", c.serverURL, err)
	}

	log.Printf("Tunnel client started: UDP %s <-> WS %s", c.localAddr, c.serverURL)

	// Track client addresses for responses
	clientMap := make(map[string]*net.UDPAddr)
	var clientMu sync.Mutex

	// Start forwarding goroutines
	go c.udpToWS(clientMap, &clientMu)
	go c.wsToUDP(clientMap, &clientMu)

	return nil
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

	if c.conn != nil {
		c.conn.Close()
	}
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

		err = c.conn.WriteMessage(websocket.BinaryMessage, buf[:n])
		if err != nil {
			log.Printf("WebSocket write error: %v", err)
			return
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

		_, data, err := c.conn.ReadMessage()
		if err != nil {
			select {
			case <-c.stopChan:
				return
			default:
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket read error: %v", err)
				}
				return
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
