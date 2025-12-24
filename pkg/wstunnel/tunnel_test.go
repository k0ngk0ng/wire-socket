package wstunnel

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestServerCreation tests server creation
func TestServerCreation(t *testing.T) {
	cfg := ServerConfig{
		ListenAddr: ":8080",
		TargetAddr: "127.0.0.1:51820",
	}
	server := NewServer(cfg)

	if server == nil {
		t.Fatal("NewServer returned nil")
	}
	if server.listenAddr != ":8080" {
		t.Errorf("Expected listenAddr :8080, got %s", server.listenAddr)
	}
	if server.targetAddr != "127.0.0.1:51820" {
		t.Errorf("Expected targetAddr 127.0.0.1:51820, got %s", server.targetAddr)
	}
}

// TestClientCreation tests client creation
func TestClientCreation(t *testing.T) {
	cfg := ClientConfig{
		LocalAddr: "127.0.0.1:51820",
		ServerURL: "ws://localhost:8080",
		Insecure:  true,
	}
	client := NewClient(cfg)

	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	if client.localAddr != "127.0.0.1:51820" {
		t.Errorf("Expected localAddr 127.0.0.1:51820, got %s", client.localAddr)
	}
	if client.serverURL != "ws://localhost:8080" {
		t.Errorf("Expected serverURL ws://localhost:8080, got %s", client.serverURL)
	}
}

// TestUDPEcho is a helper to create a UDP echo server
func createUDPEchoServer(t *testing.T, addr string) (*net.UDPConn, func()) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		t.Fatalf("Failed to resolve UDP address: %v", err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		t.Fatalf("Failed to listen on UDP: %v", err)
	}

	stopChan := make(chan struct{})

	go func() {
		buf := make([]byte, DefaultBufferSize)
		for {
			select {
			case <-stopChan:
				return
			default:
			}

			conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, remoteAddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				continue
			}

			// Echo back
			conn.WriteToUDP(buf[:n], remoteAddr)
		}
	}()

	return conn, func() {
		close(stopChan)
		conn.Close()
	}
}

// TestWebSocketUpgrade tests that the server can upgrade HTTP to WebSocket
func TestWebSocketUpgrade(t *testing.T) {
	// Create a mock UDP target
	_, stopUDP := createUDPEchoServer(t, "127.0.0.1:0")
	defer stopUDP()

	cfg := ServerConfig{
		ListenAddr: ":0",
		TargetAddr: "127.0.0.1:51820", // Won't actually connect in this test
	}
	server := NewServer(cfg)

	// Create test HTTP server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.handleWebSocket(w, r)
	}))
	defer testServer.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	// Try to connect
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer conn.Close()

	t.Log("WebSocket connection established successfully")
}

// TestEndToEndTunnel tests full tunnel functionality
func TestEndToEndTunnel(t *testing.T) {
	// 1. Create UDP echo server (simulates WireGuard)
	echoAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to resolve echo address: %v", err)
	}
	echoConn, err := net.ListenUDP("udp", echoAddr)
	if err != nil {
		t.Fatalf("Failed to create echo server: %v", err)
	}
	defer echoConn.Close()

	echoPort := echoConn.LocalAddr().(*net.UDPAddr).Port
	t.Logf("Echo server on port %d", echoPort)

	// Start echo server
	echoStop := make(chan struct{})
	go func() {
		buf := make([]byte, DefaultBufferSize)
		for {
			select {
			case <-echoStop:
				return
			default:
			}
			echoConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, addr, err := echoConn.ReadFromUDP(buf)
			if err != nil {
				continue
			}
			echoConn.WriteToUDP(buf[:n], addr)
		}
	}()
	defer close(echoStop)

	// 2. Create tunnel server
	serverCfg := ServerConfig{
		ListenAddr: "127.0.0.1:0",
		TargetAddr: echoConn.LocalAddr().String(),
	}
	tunnelServer := NewServer(serverCfg)

	// Use httptest for the server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tunnelServer.handleWebSocket(w, r)
	}))
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")
	t.Logf("Tunnel server at %s", wsURL)

	// 3. Create tunnel client
	clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to resolve client address: %v", err)
	}
	clientListener, err := net.ListenUDP("udp", clientAddr)
	if err != nil {
		t.Fatalf("Failed to create client listener: %v", err)
	}
	clientPort := clientListener.LocalAddr().(*net.UDPAddr).Port
	clientListener.Close() // Close so tunnel client can use this port

	clientCfg := ClientConfig{
		LocalAddr: clientListener.LocalAddr().String(),
		ServerURL: wsURL,
		Insecure:  true,
	}
	tunnelClient := NewClient(clientCfg)

	err = tunnelClient.Start()
	if err != nil {
		t.Fatalf("Failed to start tunnel client: %v", err)
	}
	defer tunnelClient.Stop()

	// Give it time to connect
	time.Sleep(100 * time.Millisecond)

	// 4. Send data through tunnel
	testConn, err := net.DialUDP("udp", nil, &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: clientPort,
	})
	if err != nil {
		t.Fatalf("Failed to create test UDP connection: %v", err)
	}
	defer testConn.Close()

	testData := []byte("Hello, WireGuard!")
	_, err = testConn.Write(testData)
	if err != nil {
		t.Fatalf("Failed to send test data: %v", err)
	}

	// 5. Receive echo response
	testConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, DefaultBufferSize)
	n, err := testConn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to receive echo response: %v", err)
	}

	if string(buf[:n]) != string(testData) {
		t.Errorf("Expected echo %q, got %q", testData, buf[:n])
	}

	t.Logf("End-to-end tunnel test passed! Sent and received: %q", testData)
}

// TestServerStopStart tests server stop and restart
func TestServerStopStart(t *testing.T) {
	cfg := ServerConfig{
		ListenAddr: "127.0.0.1:0",
		TargetAddr: "127.0.0.1:51820",
	}
	server := NewServer(cfg)

	// Start server in goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		server.Start()
	}()

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Stop server
	err := server.Stop()
	if err != nil {
		t.Errorf("Failed to stop server: %v", err)
	}

	wg.Wait()
}

// TestClientStopWhenNotRunning tests stopping a client that's not running
func TestClientStopWhenNotRunning(t *testing.T) {
	cfg := ClientConfig{
		LocalAddr: "127.0.0.1:51820",
		ServerURL: "ws://localhost:8080",
	}
	client := NewClient(cfg)

	// Should not error when stopping a non-running client
	err := client.Stop()
	if err != nil {
		t.Errorf("Stop on non-running client should not error: %v", err)
	}
}

// TestClientIsRunning tests the IsRunning method
func TestClientIsRunning(t *testing.T) {
	cfg := ClientConfig{
		LocalAddr: "127.0.0.1:0",
		ServerURL: "ws://localhost:8080",
	}
	client := NewClient(cfg)

	if client.IsRunning() {
		t.Error("Client should not be running before Start")
	}
}

// BenchmarkDataForwarding benchmarks data forwarding through the tunnel
func BenchmarkDataForwarding(b *testing.B) {
	// Setup echo server
	echoAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	echoConn, _ := net.ListenUDP("udp", echoAddr)
	defer echoConn.Close()

	echoStop := make(chan struct{})
	go func() {
		buf := make([]byte, DefaultBufferSize)
		for {
			select {
			case <-echoStop:
				return
			default:
			}
			echoConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, addr, err := echoConn.ReadFromUDP(buf)
			if err != nil {
				continue
			}
			echoConn.WriteToUDP(buf[:n], addr)
		}
	}()
	defer close(echoStop)

	// Setup tunnel server
	serverCfg := ServerConfig{
		ListenAddr: "127.0.0.1:0",
		TargetAddr: echoConn.LocalAddr().String(),
	}
	tunnelServer := NewServer(serverCfg)

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tunnelServer.handleWebSocket(w, r)
	}))
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	// Setup tunnel client
	clientCfg := ClientConfig{
		LocalAddr: "127.0.0.1:0",
		ServerURL: wsURL,
		Insecure:  true,
	}
	tunnelClient := NewClient(clientCfg)
	tunnelClient.Start()
	defer tunnelClient.Stop()

	time.Sleep(100 * time.Millisecond)

	// Create test connection
	clientPort := tunnelClient.udpConn.LocalAddr().(*net.UDPAddr).Port
	testConn, _ := net.DialUDP("udp", nil, &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: clientPort,
	})
	defer testConn.Close()

	testData := []byte("Benchmark test data payload for WireGuard tunnel")
	buf := make([]byte, DefaultBufferSize)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testConn.Write(testData)
		testConn.SetReadDeadline(time.Now().Add(1 * time.Second))
		testConn.Read(buf)
	}
}
