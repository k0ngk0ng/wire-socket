// Package wstunnel provides WebSocket-UDP tunnel functionality.
// This replaces the external wstunnel binary dependency.
package wstunnel

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// DefaultBufferSize is the default buffer size for UDP packets
	DefaultBufferSize = 65535

	// DefaultTimeout is the default connection timeout
	DefaultTimeout = 30 * time.Second

	// PingInterval is the WebSocket ping interval
	PingInterval = 30 * time.Second
)

// Server handles WebSocket connections and forwards data to UDP
type Server struct {
	listenAddr    string       // WebSocket listen address (e.g., ":443")
	targetAddr    string       // UDP target address (e.g., "127.0.0.1:51820")
	tlsCert       string       // TLS certificate file path
	tlsKey        string       // TLS key file path
	upgrader      websocket.Upgrader
	server        *http.Server
	mu            sync.Mutex
	running       bool
}

// ServerConfig holds server configuration
type ServerConfig struct {
	ListenAddr string // WebSocket listen address
	TargetAddr string // UDP target address (WireGuard)
	TLSCert    string // TLS certificate file (optional, for WSS)
	TLSKey     string // TLS key file (optional, for WSS)
}

// NewServer creates a new WebSocket tunnel server
func NewServer(cfg ServerConfig) *Server {
	return &Server{
		listenAddr: cfg.ListenAddr,
		targetAddr: cfg.TargetAddr,
		tlsCert:    cfg.TLSCert,
		tlsKey:     cfg.TLSKey,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  DefaultBufferSize,
			WriteBufferSize: DefaultBufferSize,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
	}
}

// Start starts the WebSocket tunnel server
func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}
	s.running = true
	s.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWebSocket)

	s.server = &http.Server{
		Addr:    s.listenAddr,
		Handler: mux,
	}

	var err error
	if s.tlsCert != "" && s.tlsKey != "" {
		log.Printf("Starting WSS tunnel server on %s -> %s", s.listenAddr, s.targetAddr)
		err = s.server.ListenAndServeTLS(s.tlsCert, s.tlsKey)
	} else {
		log.Printf("Starting WS tunnel server on %s -> %s", s.listenAddr, s.targetAddr)
		err = s.server.ListenAndServe()
	}

	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop stops the server
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

// handleWebSocket handles incoming WebSocket connections
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Connect to UDP target
	udpAddr, err := net.ResolveUDPAddr("udp", s.targetAddr)
	if err != nil {
		log.Printf("Failed to resolve UDP address %s: %v", s.targetAddr, err)
		return
	}

	udpConn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		log.Printf("Failed to connect to UDP %s: %v", s.targetAddr, err)
		return
	}
	defer udpConn.Close()

	log.Printf("New tunnel connection from %s", r.RemoteAddr)

	// Bidirectional forwarding
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	// WebSocket -> UDP
	go func() {
		defer wg.Done()
		defer cancel()
		s.wsToUDP(ctx, conn, udpConn)
	}()

	// UDP -> WebSocket
	go func() {
		defer wg.Done()
		defer cancel()
		s.udpToWS(ctx, udpConn, conn)
	}()

	wg.Wait()
	log.Printf("Tunnel connection closed from %s", r.RemoteAddr)
}

// wsToUDP forwards data from WebSocket to UDP
func (s *Server) wsToUDP(ctx context.Context, ws *websocket.Conn, udp *net.UDPConn) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, data, err := ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			return
		}

		_, err = udp.Write(data)
		if err != nil {
			log.Printf("UDP write error: %v", err)
			return
		}
	}
}

// udpToWS forwards data from UDP to WebSocket
func (s *Server) udpToWS(ctx context.Context, udp *net.UDPConn, ws *websocket.Conn) {
	buf := make([]byte, DefaultBufferSize)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		udp.SetReadDeadline(time.Now().Add(DefaultTimeout))
		n, err := udp.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			log.Printf("UDP read error: %v", err)
			return
		}

		err = ws.WriteMessage(websocket.BinaryMessage, buf[:n])
		if err != nil {
			log.Printf("WebSocket write error: %v", err)
			return
		}
	}
}

// Client handles UDP listening and forwards to WebSocket
type Client struct {
	localAddr   string          // Local UDP listen address (e.g., "127.0.0.1:51820")
	serverURL   string          // WebSocket server URL (e.g., "wss://server:443")
	conn        *websocket.Conn
	udpConn     *net.UDPConn
	mu          sync.Mutex
	running     bool
	stopChan    chan struct{}
	insecure    bool            // Skip TLS verification
}

// ClientConfig holds client configuration
type ClientConfig struct {
	LocalAddr  string // Local UDP listen address
	ServerURL  string // WebSocket server URL
	Insecure   bool   // Skip TLS verification (for self-signed certs)
}

// NewClient creates a new WebSocket tunnel client
func NewClient(cfg ClientConfig) *Client {
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
