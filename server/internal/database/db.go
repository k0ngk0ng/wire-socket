package database

import (
	"fmt"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// User represents a VPN user
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"unique;not null" json:"username"`
	Email        string    `gorm:"unique;not null" json:"email"`
	PasswordHash string    `gorm:"not null" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	IsActive     bool      `gorm:"default:true" json:"is_active"`
	IsAdmin      bool      `gorm:"default:false" json:"is_admin"`
}

// Server represents a VPN server configuration
type Server struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Name       string    `gorm:"not null" json:"name"`
	Endpoint   string    `gorm:"not null" json:"endpoint"`
	PublicKey  string    `gorm:"not null" json:"public_key"`
	PrivateKey string    `gorm:"not null" json:"-"` // Encrypted at rest
	ListenPort int       `gorm:"default:51820" json:"listen_port"`
	Subnet     string    `gorm:"not null" json:"subnet"` // e.g., 10.0.0.0/24
	DNS        string    `json:"dns"`                   // e.g., 1.1.1.1,8.8.8.8
	CreatedAt  time.Time `json:"created_at"`
}

// AllocatedIP tracks IP assignments to users
type AllocatedIP struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"not null" json:"user_id"`
	ServerID   uint      `gorm:"not null" json:"server_id"`
	IPAddress  string    `gorm:"not null" json:"ip_address"`
	PublicKey  string    `gorm:"not null" json:"public_key"` // User's WG public key
	AllocatedAt time.Time `json:"allocated_at"`
	LastSeen   *time.Time `json:"last_seen"`

	User   User   `gorm:"foreignKey:UserID" json:"-"`
	Server Server `gorm:"foreignKey:ServerID" json:"-"`
}

// Session represents an auth session (for JWT revocation)
type Session struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"not null" json:"user_id"`
	TokenHash string    `gorm:"not null" json:"token_hash"`
	ExpiresAt time.Time `gorm:"not null" json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

// Route represents a route for VPN
// Routes are both pushed to clients AND applied on server side
type Route struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CIDR      string    `gorm:"not null;unique" json:"cidr"` // e.g., "192.168.1.0/24"
	Gateway   string    `json:"gateway,omitempty"`           // Next hop (optional, for server-side routing)
	Device    string    `json:"device,omitempty"`            // Interface (optional, defaults to wg device)
	Comment   string    `json:"comment"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	PushToClient bool   `gorm:"default:true" json:"push_to_client"` // Push this route to VPN clients
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NATRuleType defines the type of NAT rule
type NATRuleType string

const (
	NATTypeMasquerade NATRuleType = "masquerade"
	NATTypeSNAT       NATRuleType = "snat"
	NATTypeDNAT       NATRuleType = "dnat"
	NATTypeTCPMSS     NATRuleType = "tcpmss"
)

// NATRule represents a NAT/iptables rule
type NATRule struct {
	ID        uint        `gorm:"primaryKey" json:"id"`
	Type      NATRuleType `gorm:"not null" json:"type"` // masquerade, snat, dnat
	Comment   string      `json:"comment"`
	Enabled   bool        `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`

	// For MASQUERADE
	Interface string `json:"interface,omitempty"` // Outbound interface (e.g., "eth0")

	// For SNAT
	Source      string `json:"source,omitempty"`      // Source CIDR
	Destination string `json:"destination,omitempty"` // Destination CIDR
	ToSource    string `json:"to_source,omitempty"`   // SNAT to this IP

	// For DNAT
	Protocol      string `json:"protocol,omitempty"`       // tcp or udp
	Port          int    `json:"port,omitempty"`           // External port
	ToDestination string `json:"to_destination,omitempty"` // Forward to address:port

	// For TCPMSS (MSS clamping to prevent MTU issues)
	MSS int `json:"mss,omitempty"` // MSS value (e.g., 1360)
}

// Group represents a user group for route assignment
type Group struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"unique;not null" json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UserGroup is a many-to-many join table between users and groups
type UserGroup struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"not null;uniqueIndex:idx_user_group" json:"user_id"`
	GroupID   uint      `gorm:"not null;uniqueIndex:idx_user_group" json:"group_id"`
	CreatedAt time.Time `json:"created_at"`

	User  User  `gorm:"foreignKey:UserID" json:"-"`
	Group Group `gorm:"foreignKey:GroupID" json:"-"`
}

// RouteGroup is a many-to-many join table between routes and groups
type RouteGroup struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	RouteID   uint      `gorm:"not null;uniqueIndex:idx_route_group" json:"route_id"`
	GroupID   uint      `gorm:"not null;uniqueIndex:idx_route_group" json:"group_id"`
	CreatedAt time.Time `json:"created_at"`

	Route Route `gorm:"foreignKey:RouteID" json:"-"`
	Group Group `gorm:"foreignKey:GroupID" json:"-"`
}

// DB holds the database connection
type DB struct {
	*gorm.DB
}

// NewDB initializes and returns a new database connection
func NewDB(dbPath string) (*DB, error) {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Auto-migrate schemas
	if err := db.AutoMigrate(&User{}, &Server{}, &AllocatedIP{}, &Session{}, &Route{}, &NATRule{}, &Group{}, &UserGroup{}, &RouteGroup{}); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	// Add unique index for IP allocation
	if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_server_ip ON allocated_ips(server_id, ip_address)").Error; err != nil {
		return nil, fmt.Errorf("failed to create index: %w", err)
	}

	return &DB{db}, nil
}

// CreateDefaultServer creates a default server configuration if none exists
func (db *DB) CreateDefaultServer(name, endpoint, subnet, dns string) error {
	var count int64
	if err := db.Model(&Server{}).Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		return nil // Server already exists
	}

	server := &Server{
		Name:       name,
		Endpoint:   endpoint,
		Subnet:     subnet,
		DNS:        dns,
		ListenPort: 51820,
	}

	return db.Create(server).Error
}
