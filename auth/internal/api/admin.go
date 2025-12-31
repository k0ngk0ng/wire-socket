package api

import (
	"net/http"
	"strconv"
	"wire-socket-auth/internal/database"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// AdminHandler handles admin API endpoints
type AdminHandler struct {
	db *database.DB
}

// NewAdminHandler creates a new AdminHandler
func NewAdminHandler(db *database.DB) *AdminHandler {
	return &AdminHandler{db: db}
}

// ============ User Management ============

// ListUsers returns all users
func (h *AdminHandler) ListUsers(c *gin.Context) {
	var users []database.User
	if err := h.db.Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch users"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": users})
}

// GetUser returns a specific user
func (h *AdminHandler) GetUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var user database.User
	if err := h.db.First(&user, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": user})
}

// CreateUserRequest for creating a user
type CreateUserRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email"`
	Password string `json:"password" binding:"required,min=6"`
	IsAdmin  bool   `json:"is_admin"`
}

// CreateUser creates a new user
func (h *AdminHandler) CreateUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Hash password
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	user := database.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(passwordHash),
		IsActive:     true,
		IsAdmin:      req.IsAdmin,
	}

	if err := h.db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"user": user})
}

// UpdateUserRequest for updating a user
type UpdateUserRequest struct {
	Username *string `json:"username"`
	Email    *string `json:"email"`
	Password *string `json:"password"`
	IsActive *bool   `json:"is_active"`
	IsAdmin  *bool   `json:"is_admin"`
}

// UpdateUser updates a user
func (h *AdminHandler) UpdateUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var user database.User
	if err := h.db.First(&user, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Username != nil {
		user.Username = *req.Username
	}
	if req.Email != nil {
		user.Email = *req.Email
	}
	if req.Password != nil {
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
			return
		}
		user.PasswordHash = string(passwordHash)
	}
	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}
	if req.IsAdmin != nil {
		user.IsAdmin = *req.IsAdmin
	}

	if err := h.db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": user})
}

// DeleteUser deletes a user
func (h *AdminHandler) DeleteUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var user database.User
	if err := h.db.First(&user, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	if err := h.db.Delete(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user deleted successfully"})
}

// ============ Tunnel Management ============

// ListTunnels returns all tunnels (admin view)
func (h *AdminHandler) ListTunnels(c *gin.Context) {
	var tunnels []database.Tunnel
	if err := h.db.Find(&tunnels).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tunnels"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tunnels": tunnels})
}

// GetTunnel returns a specific tunnel
func (h *AdminHandler) GetTunnel(c *gin.Context) {
	id := c.Param("id")

	var tunnel database.Tunnel
	if err := h.db.First(&tunnel, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tunnel not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tunnel": tunnel})
}

// UpdateTunnelRequest for updating a tunnel
type UpdateTunnelRequest struct {
	Name     *string `json:"name"`
	URL      *string `json:"url"`
	Region   *string `json:"region"`
	IsActive *bool   `json:"is_active"`
}

// UpdateTunnel updates a tunnel
func (h *AdminHandler) UpdateTunnel(c *gin.Context) {
	id := c.Param("id")

	var tunnel database.Tunnel
	if err := h.db.First(&tunnel, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tunnel not found"})
		return
	}

	var req UpdateTunnelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != nil {
		tunnel.Name = *req.Name
	}
	if req.URL != nil {
		tunnel.URL = *req.URL
	}
	if req.Region != nil {
		tunnel.Region = *req.Region
	}
	if req.IsActive != nil {
		tunnel.IsActive = *req.IsActive
	}

	if err := h.db.Save(&tunnel).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update tunnel"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tunnel": tunnel})
}

// DeleteTunnel deletes a tunnel
func (h *AdminHandler) DeleteTunnel(c *gin.Context) {
	id := c.Param("id")

	var tunnel database.Tunnel
	if err := h.db.First(&tunnel, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tunnel not found"})
		return
	}

	// Delete access records first
	h.db.Where("tunnel_id = ?", id).Delete(&database.UserTunnelAccess{})

	if err := h.db.Delete(&tunnel).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete tunnel"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "tunnel deleted successfully"})
}

// ============ User-Tunnel Access Management ============

// GetUserTunnelAccess returns tunnels a user can access
func (h *AdminHandler) GetUserTunnelAccess(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	tunnels, err := h.db.GetUserAllowedTunnels(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch access"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tunnels": tunnels})
}

// SetUserTunnelAccessRequest for setting user tunnel access
type SetUserTunnelAccessRequest struct {
	TunnelIDs []string `json:"tunnel_ids"`
}

// SetUserTunnelAccess sets which tunnels a user can access
func (h *AdminHandler) SetUserTunnelAccess(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var req SetUserTunnelAccessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Delete existing access
	h.db.Where("user_id = ?", id).Delete(&database.UserTunnelAccess{})

	// Add new access
	for _, tunnelID := range req.TunnelIDs {
		access := database.UserTunnelAccess{
			UserID:   uint(id),
			TunnelID: tunnelID,
		}
		h.db.Create(&access)
	}

	c.JSON(http.StatusOK, gin.H{"message": "access updated"})
}
