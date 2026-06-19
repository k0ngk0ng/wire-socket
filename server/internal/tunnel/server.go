// Package tunnel provides WebSocket-UDP tunnel server functionality.
// This replaces the external wstunnel binary dependency.
package tunnel

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

const (
	// DefaultBufferSize is the default buffer size for UDP packets
	DefaultBufferSize = 65535

	// DefaultTimeout is the default connection timeout
	DefaultTimeout = 30 * time.Second

	// PingInterval is the interval between ping messages
	PingInterval = 30 * time.Second

	// PongTimeout is the timeout for pong response
	PongTimeout = 10 * time.Second

	// ReadTimeout is the timeout for read operations
	// Should be longer than PingInterval + PongTimeout to allow for ping/pong cycle
	ReadTimeout = 60 * time.Second
)

// Server handles WebSocket connections and forwards data to UDP
type Server struct {
	listenAddr string // WebSocket listen address (e.g., ":443")
	targetAddr string // UDP target address (e.g., "127.0.0.1:51820")
	pathPrefix string // Path prefix for WebSocket upgrade (e.g., "/tunnel")
	tlsCert    string // TLS certificate file path
	tlsKey     string // TLS key file path
	upgrader   websocket.Upgrader
	server     *http.Server
	mu         sync.Mutex
	running    bool
}

// Config holds server configuration
type Config struct {
	ListenAddr string // WebSocket listen address
	TargetAddr string // UDP target address (WireGuard)
	PathPrefix string // Path prefix for WebSocket upgrade (default: "/")
	TLSCert    string // TLS certificate file (optional, for WSS)
	TLSKey     string // TLS key file (optional, for WSS)
}

// NewServer creates a new WebSocket tunnel server
func NewServer(cfg Config) *Server {
	pathPrefix := cfg.PathPrefix
	if pathPrefix == "" {
		pathPrefix = "/"
	}
	// Ensure path starts with /
	if pathPrefix[0] != '/' {
		pathPrefix = "/" + pathPrefix
	}

	return &Server{
		listenAddr: cfg.ListenAddr,
		targetAddr: cfg.TargetAddr,
		pathPrefix: pathPrefix,
		tlsCert:    cfg.TLSCert,
		tlsKey:     cfg.TLSKey,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  DefaultBufferSize,
			WriteBufferSize: DefaultBufferSize,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
	}
}

// Start starts the WebSocket tunnel server (blocking)
func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}
	s.running = true
	s.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc(s.pathPrefix, s.handleWebSocket)

	s.server = &http.Server{
		Addr:    s.listenAddr,
		Handler: mux,
	}

	var err error
	if s.tlsCert != "" && s.tlsKey != "" {
		log.Printf("Starting WSS tunnel server on %s%s -> %s", s.listenAddr, s.pathPrefix, s.targetAddr)
		err = s.server.ListenAndServeTLS(s.tlsCert, s.tlsKey)
	} else {
		log.Printf("Starting WS tunnel server on %s%s -> %s", s.listenAddr, s.pathPrefix, s.targetAddr)
		err = s.server.ListenAndServe()
	}

	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// StartAsync starts the server in a goroutine
func (s *Server) StartAsync() error {
	errChan := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil {
			errChan <- err
		}
	}()

	// Wait a bit to see if there's an immediate error
	select {
	case err := <-errChan:
		return err
	case <-time.After(100 * time.Millisecond):
		return nil
	}
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

// IsRunning returns whether the server is running
func (s *Server) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// handleWebSocket handles incoming WebSocket connections
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()
	var writeMu sync.Mutex

	// Set up ping/pong handling
	// Client sends ping every 30s - we must extend read deadline on receiving ping
	// The default PingHandler auto-responds with pong, but does NOT extend the deadline
	// We override it to both respond with pong AND extend the read deadline
	conn.SetReadDeadline(time.Now().Add(ReadTimeout))
	conn.SetPingHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(ReadTimeout))
		// Send pong response (same as default handler behavior)
		writeMu.Lock()
		err := conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(PongTimeout))
		writeMu.Unlock()
		if err == websocket.ErrCloseSent {
			return nil
		} else if e, ok := err.(net.Error); ok && e.Timeout() {
			return nil
		}
		return err
	})
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(ReadTimeout))
		return nil
	})

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
	wg.Add(3)

	// Server-side ping loop: actively ping client to keep connection alive
	go func() {
		defer wg.Done()
		defer cancel()
		s.pingLoop(ctx, conn, &writeMu)
	}()

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
		s.udpToWS(ctx, udpConn, conn, &writeMu)
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

		// ReadMessage will also process ping frames and send pong responses
		// The read deadline is extended by the pong handler when we receive data
		_, data, err := ws.ReadMessage()
		if err != nil {
			// Check if it's a timeout
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Printf("WebSocket read timeout, closing connection")
			} else if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			return
		}

		// Extend read deadline after successful read
		ws.SetReadDeadline(time.Now().Add(ReadTimeout))

		_, err = udp.Write(data)
		if err != nil {
			log.Printf("UDP write error: %v", err)
			return
		}
	}
}

// udpToWS forwards data from UDP to WebSocket
func (s *Server) udpToWS(ctx context.Context, udp *net.UDPConn, ws *websocket.Conn, writeMu *sync.Mutex) {
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

		writeMu.Lock()
		err = ws.WriteMessage(websocket.BinaryMessage, buf[:n])
		writeMu.Unlock()
		if err != nil {
			log.Printf("WebSocket write error: %v", err)
			return
		}
	}
}

// pingLoop sends periodic ping messages from server to client to keep the connection alive.
// This ensures bidirectional keepalive: client pings server AND server pings client.
func (s *Server) pingLoop(ctx context.Context, ws *websocket.Conn, writeMu *sync.Mutex) {
	ticker := time.NewTicker(PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deadline := time.Now().Add(PongTimeout)
			writeMu.Lock()
			err := ws.WriteControl(websocket.PingMessage, []byte{}, deadline)
			writeMu.Unlock()
			if err != nil {
				// Connection is likely dead
				log.Printf("Server ping failed: %v", err)
				return
			}
		}
	}
}
