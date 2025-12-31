package database

import (
	"log"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ============ Auth Service Models ============

// AuthUser represents a user account (managed by auth service)
type AuthUser struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"column:username;uniqueIndex;not null" json:"username"`
	Email        string    `gorm:"column:email;uniqueIndex" json:"email"`
	PasswordHash string    `gorm:"column:password_hash;not null" json:"-"`
	IsActive     bool      `gorm:"column:is_active;default:true" json:"is_active"`
	IsAdmin      bool      `gorm:"column:is_admin;default:false" json:"is_admin"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName overrides the table name for AuthUser
func (AuthUser) TableName() string {
	return "users"
}

// Tunnel represents a registered tunnel node
type Tunnel struct {
	ID          string    `gorm:"primaryKey" json:"id"`                    // e.g., "hk-01"
	Name        string    `gorm:"column:name;not null" json:"name"`        // e.g., "Hong Kong"
	URL         string    `gorm:"column:url;not null" json:"url"`          // Public URL
	InternalURL string    `gorm:"column:internal_url" json:"internal_url"` // For auth communication
	Region      string    `gorm:"column:region" json:"region"`
	TokenHash   string    `gorm:"column:token_hash;not null" json:"-"` // Pre-shared secret hash
	IsActive    bool      `gorm:"column:is_active;default:true" json:"is_active"`
	LastSeen    time.Time `gorm:"column:last_seen" json:"last_seen"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UserTunnelAccess defines which tunnels a user can access
type UserTunnelAccess struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"column:user_id;not null;index" json:"user_id"`
	TunnelID  string    `gorm:"column:tunnel_id;not null;index" json:"tunnel_id"`
	CreatedAt time.Time `json:"created_at"`

	User   AuthUser `gorm:"foreignKey:UserID" json:"-"`
	Tunnel Tunnel   `gorm:"foreignKey:TunnelID" json:"-"`
}

// AuthSession tracks active user sessions for auth service
type AuthSession struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"column:user_id;not null" json:"user_id"`
	TokenHash string    `gorm:"column:token_hash;not null" json:"token_hash"`
	ExpiresAt time.Time `gorm:"column:expires_at;not null" json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`

	User AuthUser `gorm:"foreignKey:UserID" json:"-"`
}

// TableName overrides the table name for AuthSession
func (AuthSession) TableName() string {
	return "auth_sessions"
}

// AuthDB wraps gorm.DB for auth service
type AuthDB struct {
	*gorm.DB
}

// NewAuthDB creates a new auth service database connection
func NewAuthDB(dbPath string) (*AuthDB, error) {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}

	return &AuthDB{DB: db}, nil
}

// AutoMigrate creates/updates auth service database tables
func (db *AuthDB) AutoMigrate() error {
	return db.DB.AutoMigrate(
		&AuthUser{},
		&Tunnel{},
		&UserTunnelAccess{},
		&AuthSession{},
	)
}

// InitAdmin creates default admin user if not exists
func (db *AuthDB) InitAdmin(passwordHash string) error {
	var count int64
	db.Model(&AuthUser{}).Where("is_admin = ?", true).Count(&count)
	if count > 0 {
		log.Println("Admin user already exists")
		return nil
	}

	admin := AuthUser{
		Username:     "admin",
		Email:        "admin@localhost",
		PasswordHash: passwordHash,
		IsActive:     true,
		IsAdmin:      true,
	}

	if err := db.Create(&admin).Error; err != nil {
		return err
	}

	log.Println("Created default admin user: admin")
	return nil
}

// GetUserAllowedTunnels returns tunnel IDs a user can access
func (db *AuthDB) GetUserAllowedTunnels(userID uint) ([]string, error) {
	var accesses []UserTunnelAccess
	if err := db.Where("user_id = ?", userID).Find(&accesses).Error; err != nil {
		return nil, err
	}

	// If no explicit access, allow all active tunnels (for backward compatibility)
	if len(accesses) == 0 {
		var tunnels []Tunnel
		if err := db.Where("is_active = ?", true).Find(&tunnels).Error; err != nil {
			return nil, err
		}
		ids := make([]string, len(tunnels))
		for i, t := range tunnels {
			ids[i] = t.ID
		}
		return ids, nil
	}

	ids := make([]string, len(accesses))
	for i, a := range accesses {
		ids[i] = a.TunnelID
	}
	return ids, nil
}
