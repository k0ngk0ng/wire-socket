package sdk

import (
	"crypto/tls"
	"log"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Tunnel constants
	tunnelBufferSize   = 65535
	tunnelTimeout      = 30 * time.Second
	tunnelPingInterval = 30 * time.Second
	tunnelPongTimeout  = 10 * time.Second
	tunnelReadTimeout  = 45 * time.Second
)

// tunnelClient handles WebSocket-UDP tunnel with keepalive support
type tunnelClient struct {
	serverURL string
	insecure  bool
	conn      *websocket.Conn
	udpConn   *net.UDPConn
	running   bool
	stopChan  chan struct{}
	port      int
	mu        sync.Mutex
	connMu    sync.RWMutex
	writeMu   sync.Mutex
	lastPong  time.Time
}

func newTunnelClient(serverURL string, insecure bool) *tunnelClient {
	return &tunnelClient{
		serverURL: serverURL,
		insecure:  insecure,
		stopChan:  make(chan struct{}),
	}
}

func (t *tunnelClient) Start() error {
	// Listen on dynamic port
	udpAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	var err error
	t.udpConn, err = net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	t.port = t.udpConn.LocalAddr().(*net.UDPAddr).Port

	// Connect to WebSocket
	if err := t.connectWebSocket(); err != nil {
		t.udpConn.Close()
		return err
	}

	t.mu.Lock()
	t.running = true
	t.mu.Unlock()

	// Track client addresses for responses
	clientMap := make(map[string]*net.UDPAddr)
	var clientMu sync.Mutex

	go t.forwardUDPToWS(clientMap, &clientMu)
	go t.forwardWSToUDP(clientMap, &clientMu)
	go t.pingLoop()

	return nil
}

func (t *tunnelClient) markAlive() {
	t.connMu.Lock()
	t.lastPong = time.Now()
	t.connMu.Unlock()
}

func (t *tunnelClient) writeControl(conn *websocket.Conn, messageType int, data []byte, deadline time.Time) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	t.connMu.RLock()
	currentConn := t.conn
	t.connMu.RUnlock()
	if currentConn != conn {
		return nil
	}

	conn.SetWriteDeadline(deadline)
	err := conn.WriteControl(messageType, data, deadline)
	conn.SetWriteDeadline(time.Time{})
	return err
}

func (t *tunnelClient) connectWebSocket() error {
	dialer := websocket.Dialer{
		HandshakeTimeout: tunnelTimeout,
	}
	if t.insecure {
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	conn, _, err := dialer.Dial(t.serverURL, nil)
	if err != nil {
		return err
	}

	t.connMu.Lock()
	t.conn = conn
	t.lastPong = time.Now()
	t.connMu.Unlock()

	// Set pong handler - tracks responses to our pings
	conn.SetPongHandler(func(string) error {
		t.markAlive()
		return nil
	})

	// Set ping handler - server also sends pings to us
	// Default handler auto-responds with pong, but we also update lastPong
	// to reflect that the connection is alive (server reached us)
	conn.SetPingHandler(func(appData string) error {
		t.markAlive()
		// Send pong response (same as default handler behavior)
		err := t.writeControl(conn, websocket.PongMessage, []byte(appData), time.Now().Add(tunnelPongTimeout))
		if err == websocket.ErrCloseSent {
			return nil
		} else if e, ok := err.(net.Error); ok && e.Timeout() {
			return nil
		}
		return err
	})

	return nil
}

func (t *tunnelClient) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running {
		return
	}

	t.running = false
	close(t.stopChan)

	t.connMu.Lock()
	if t.conn != nil {
		t.conn.Close()
		t.conn = nil
	}
	t.connMu.Unlock()

	if t.udpConn != nil {
		t.udpConn.Close()
	}
}

func (t *tunnelClient) IsRunning() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}

func (t *tunnelClient) LocalPort() int {
	return t.port
}

func (t *tunnelClient) forwardUDPToWS(clientMap map[string]*net.UDPAddr, mu *sync.Mutex) {
	buf := make([]byte, tunnelBufferSize)

	for {
		select {
		case <-t.stopChan:
			return
		default:
		}

		t.udpConn.SetReadDeadline(time.Now().Add(time.Second))
		n, addr, err := t.udpConn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-t.stopChan:
				return
			default:
				continue
			}
		}

		mu.Lock()
		clientMap["last"] = addr
		mu.Unlock()

		t.connMu.RLock()
		conn := t.conn
		t.connMu.RUnlock()

		if conn != nil {
			t.writeMu.Lock()
			err := conn.WriteMessage(websocket.BinaryMessage, buf[:n])
			t.writeMu.Unlock()
			if err != nil {
				log.Printf("WebSocket write error: %v", err)
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
}

func (t *tunnelClient) forwardWSToUDP(clientMap map[string]*net.UDPAddr, mu *sync.Mutex) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("WebSocket read recovered from panic: %v", r)
		}
	}()

	for {
		select {
		case <-t.stopChan:
			return
		default:
		}

		t.mu.Lock()
		running := t.running
		t.mu.Unlock()

		// Exit if not running
		if !running {
			return
		}

		t.connMu.RLock()
		conn := t.conn
		t.connMu.RUnlock()

		if conn == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Set read deadline to prevent blocking forever
		conn.SetReadDeadline(time.Now().Add(tunnelReadTimeout))

		_, data, err := conn.ReadMessage()
		if err != nil {
			select {
			case <-t.stopChan:
				return
			default:
				// Check if connection was closed
				t.connMu.RLock()
				currentConn := t.conn
				t.connMu.RUnlock()
				if currentConn == nil {
					// Connection was closed, exit gracefully
					return
				}

				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket read error: %v", err)
				}
				// Connection error, exit and let reconnect handle it
				return
			}
		}

		t.markAlive()

		mu.Lock()
		clientAddr := clientMap["last"]
		mu.Unlock()

		if clientAddr != nil {
			_, err = t.udpConn.WriteToUDP(data, clientAddr)
			if err != nil {
				log.Printf("UDP write error: %v", err)
			}
		}
	}
}

func (t *tunnelClient) pingLoop() {
	ticker := time.NewTicker(tunnelPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopChan:
			return
		case <-ticker.C:
			t.connMu.RLock()
			conn := t.conn
			lastPong := t.lastPong
			t.connMu.RUnlock()

			if conn == nil {
				continue
			}

			// Check if we received pong recently
			timeSinceLastPong := time.Since(lastPong)
			if timeSinceLastPong > tunnelPingInterval+tunnelPongTimeout {
				log.Printf("No pong received for %v, connection may be dead", timeSinceLastPong)
				t.connMu.Lock()
				if t.conn != nil {
					t.conn.Close()
					t.conn = nil
				}
				t.connMu.Unlock()

				// Mark as not running - connectionMonitor will handle reconnect
				t.mu.Lock()
				t.running = false
				t.mu.Unlock()
				return
			}

			// Send ping
			deadline := time.Now().Add(tunnelPongTimeout)
			if err := t.writeControl(conn, websocket.PingMessage, []byte{}, deadline); err != nil {
				log.Printf("Failed to send ping: %v", err)
			}
		}
	}
}
