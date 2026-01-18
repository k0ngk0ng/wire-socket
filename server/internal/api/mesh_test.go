package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestNewMeshHandler(t *testing.T) {
	handler := NewMeshHandler(nil, "test-token")

	if handler == nil {
		t.Fatal("expected non-nil MeshHandler")
	}
	if handler.meshToken != "test-token" {
		t.Errorf("expected meshToken 'test-token', got '%s'", handler.meshToken)
	}
}

func TestMeshTokenMiddleware_ValidToken(t *testing.T) {
	handler := NewMeshHandler(nil, "valid-token")

	router := gin.New()
	router.Use(handler.MeshTokenMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Mesh-Token", "valid-token")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestMeshTokenMiddleware_InvalidToken(t *testing.T) {
	handler := NewMeshHandler(nil, "valid-token")

	router := gin.New()
	router.Use(handler.MeshTokenMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Mesh-Token", "invalid-token")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestMeshTokenMiddleware_MissingToken(t *testing.T) {
	handler := NewMeshHandler(nil, "valid-token")

	router := gin.New()
	router.Use(handler.MeshTokenMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	// No X-Mesh-Token header
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestGetStatus_MeshDisabled(t *testing.T) {
	handler := NewMeshHandler(nil, "token")

	router := gin.New()
	router.GET("/mesh/status", handler.GetStatus)

	req := httptest.NewRequest("GET", "/mesh/status", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Response should indicate mesh is disabled
	body := w.Body.String()
	if !contains(body, "\"enabled\":false") {
		t.Errorf("expected response to contain 'enabled:false', got '%s'", body)
	}
}

func TestListPeers_MeshDisabled(t *testing.T) {
	handler := NewMeshHandler(nil, "token")

	router := gin.New()
	router.GET("/mesh/peers", handler.ListPeers)

	req := httptest.NewRequest("GET", "/mesh/peers", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 when mesh disabled, got %d", w.Code)
	}
}

func TestListExitRoutes_MeshDisabled(t *testing.T) {
	handler := NewMeshHandler(nil, "token")

	router := gin.New()
	router.GET("/mesh/exit-routes", handler.ListExitRoutes)

	req := httptest.NewRequest("GET", "/mesh/exit-routes", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 when mesh disabled, got %d", w.Code)
	}
}

func TestTriggerSync_MeshDisabled(t *testing.T) {
	handler := NewMeshHandler(nil, "token")

	router := gin.New()
	router.POST("/mesh/sync", handler.TriggerSync)

	req := httptest.NewRequest("POST", "/mesh/sync", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 when mesh disabled, got %d", w.Code)
	}
}

func TestHandshake_MeshDisabled(t *testing.T) {
	handler := NewMeshHandler(nil, "token")

	router := gin.New()
	router.GET("/mesh/handshake", handler.Handshake)

	req := httptest.NewRequest("GET", "/mesh/handshake", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 when mesh disabled, got %d", w.Code)
	}
}

func TestGetSyncData_MeshDisabled(t *testing.T) {
	handler := NewMeshHandler(nil, "token")

	router := gin.New()
	router.GET("/mesh/sync-data", handler.GetSyncData)

	req := httptest.NewRequest("GET", "/mesh/sync-data", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 when mesh disabled, got %d", w.Code)
	}
}

func TestGetMeshRoutes_NilManager(t *testing.T) {
	handler := NewMeshHandler(nil, "token")

	routes := handler.GetMeshRoutes()

	if routes != nil {
		t.Errorf("expected nil routes when mesh disabled, got %v", routes)
	}
}

func TestAddPeerRequest(t *testing.T) {
	req := AddPeerRequest{
		TunnelURL:   "wss://peer.example.com/",
		APIEndpoint: "https://peer.example.com:8080",
	}

	if req.TunnelURL != "wss://peer.example.com/" {
		t.Errorf("expected TunnelURL 'wss://peer.example.com/', got '%s'", req.TunnelURL)
	}
	if req.APIEndpoint != "https://peer.example.com:8080" {
		t.Errorf("expected APIEndpoint 'https://peer.example.com:8080', got '%s'", req.APIEndpoint)
	}
}

func TestAddExitRouteRequest(t *testing.T) {
	req := AddExitRouteRequest{
		CIDR:     "192.168.1.0/24",
		Comment:  "Office network",
		Priority: 100,
	}

	if req.CIDR != "192.168.1.0/24" {
		t.Errorf("expected CIDR '192.168.1.0/24', got '%s'", req.CIDR)
	}
	if req.Comment != "Office network" {
		t.Errorf("expected Comment 'Office network', got '%s'", req.Comment)
	}
	if req.Priority != 100 {
		t.Errorf("expected Priority 100, got %d", req.Priority)
	}
}

func TestRegisterRequest(t *testing.T) {
	req := RegisterRequest{
		Name:       "exit-jp",
		PublicKey:  "pubkey123",
		MeshIP:     "10.254.0.2",
		TunnelURL:  "wss://exit-jp.example.com/",
		ExitRoutes: []string{"192.168.1.0/24", "10.10.0.0/16"},
	}

	if req.Name != "exit-jp" {
		t.Errorf("expected Name 'exit-jp', got '%s'", req.Name)
	}
	if len(req.ExitRoutes) != 2 {
		t.Errorf("expected 2 exit routes, got %d", len(req.ExitRoutes))
	}
}

// Helper function
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
