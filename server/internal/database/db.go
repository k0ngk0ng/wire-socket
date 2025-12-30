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
	Username     string    `gorm:"column:username;unique;not null" json:"username"`
	Email        string    `gorm:"column:email;unique;not null" json:"email"`
	PasswordHash string    `gorm:"column:password_hash;not null" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	IsActive     bool      `gorm:"column:is_active;default:true" json:"is_active"`
	IsAdmin      bool      `gorm:"column:is_admin;default:false" json:"is_admin"`
}

// Server represents a VPN server configuration
type Server struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Name       string    `gorm:"column:name;not null" json:"name"`
	Endpoint   string    `gorm:"column:endpoint;not null" json:"endpoint"`
	PublicKey  string    `gorm:"column:public_key;not null" json:"public_key"`
	PrivateKey string    `gorm:"column:private_key;not null" json:"-"` // Encrypted at rest
	ListenPort int       `gorm:"column:listen_port;default:51820" json:"listen_port"`
	Subnet     string    `gorm:"column:subnet;not null" json:"subnet"` // e.g., 10.0.0.0/24
	DNS        string    `gorm:"column:dns" json:"dns"`                // e.g., 1.1.1.1,8.8.8.8
	CreatedAt  time.Time `json:"created_at"`
}

// AllocatedIP tracks IP assignments to users
type AllocatedIP struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	UserID      uint       `gorm:"column:user_id;not null" json:"user_id"`
	ServerID    uint       `gorm:"column:server_id;not null" json:"server_id"`
	IPAddress   string     `gorm:"column:ip_address;not null" json:"ip_address"`
	PublicKey   string     `gorm:"column:public_key;not null" json:"public_key"` // User's WG public key
	AllocatedAt time.Time  `gorm:"column:allocated_at" json:"allocated_at"`
	LastSeen    *time.Time `gorm:"column:last_seen" json:"last_seen"`

	User   User   `gorm:"foreignKey:UserID" json:"-"`
	Server Server `gorm:"foreignKey:ServerID" json:"-"`
}

// Session represents an auth session (for JWT revocation)
type Session struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"column:user_id;not null" json:"user_id"`
	TokenHash string    `gorm:"column:token_hash;not null" json:"token_hash"`
	ExpiresAt time.Time `gorm:"column:expires_at;not null" json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

// Route represents a route for VPN
// Routes can be pushed to clients AND/OR applied on server side
type Route struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	CIDR          string    `gorm:"column:cidr;not null;unique" json:"cidr"` // e.g., "192.168.1.0/24"
	Gateway       string    `gorm:"column:gateway" json:"gateway,omitempty"` // Next hop (optional, for server-side routing)
	Device        string    `gorm:"column:device" json:"device,omitempty"`   // Interface (optional, defaults to wg device)
	Metric        int       `gorm:"column:metric" json:"metric,omitempty"`   // Route priority (lower = higher priority)
	Comment       string    `gorm:"column:comment" json:"comment"`
	Enabled       bool      `gorm:"column:enabled;default:true" json:"enabled"`
	PushToClient  bool      `gorm:"column:push_to_client;default:true" json:"push_to_client"`   // Push this route to VPN clients
	ApplyOnServer bool      `gorm:"column:apply_on_server;default:false" json:"apply_on_server"` // Apply this route on server
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
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
	Type      NATRuleType `gorm:"column:type;not null" json:"type"` // masquerade, snat, dnat
	Comment   string      `gorm:"column:comment" json:"comment"`
	Enabled   bool        `gorm:"column:enabled;default:true" json:"enabled"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`

	// For MASQUERADE
	Interface string `gorm:"column:interface" json:"interface,omitempty"` // Outbound interface (e.g., "eth0")

	// For SNAT
	Source      string `gorm:"column:source" json:"source,omitempty"`           // Source CIDR
	Destination string `gorm:"column:destination" json:"destination,omitempty"` // Destination CIDR
	ToSource    string `gorm:"column:to_source" json:"to_source,omitempty"`     // SNAT to this IP

	// For DNAT
	Protocol      string `gorm:"column:protocol" json:"protocol,omitempty"`             // tcp or udp
	Port          int    `gorm:"column:port" json:"port,omitempty"`                     // External port
	ToDestination string `gorm:"column:to_destination" json:"to_destination,omitempty"` // Forward to address:port

	// For TCPMSS (MSS clamping to prevent MTU issues)
	MSS int `gorm:"column:mss" json:"mss,omitempty"` // MSS value (e.g., 1360)
}

// Group represents a user group for route assignment
type Group struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"column:name;unique;not null" json:"name"`
	Description string    `gorm:"column:description" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UserGroup is a many-to-many join table between users and groups
type UserGroup struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"column:user_id;not null;uniqueIndex:idx_user_group" json:"user_id"`
	GroupID   uint      `gorm:"column:group_id;not null;uniqueIndex:idx_user_group" json:"group_id"`
	CreatedAt time.Time `json:"created_at"`

	User  User  `gorm:"foreignKey:UserID" json:"-"`
	Group Group `gorm:"foreignKey:GroupID" json:"-"`
}

// RouteGroup is a many-to-many join table between routes and groups
type RouteGroup struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	RouteID   uint      `gorm:"column:route_id;not null;uniqueIndex:idx_route_group" json:"route_id"`
	GroupID   uint      `gorm:"column:group_id;not null;uniqueIndex:idx_route_group" json:"group_id"`
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

	// Run column rename migrations before AutoMigrate
	if err := migrateColumnNames(db); err != nil {
		return nil, fmt.Errorf("failed to migrate column names: %w", err)
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

// migrateColumnNames fixes column names that were incorrectly named by GORM
// (e.g., CIDR -> c_i_d_r instead of cidr)
func migrateColumnNames(db *gorm.DB) error {
	// Check if routes table has old c_id_r column
	var count int64
	err := db.Raw("SELECT COUNT(*) FROM pragma_table_info('routes') WHERE name = 'c_id_r'").Scan(&count).Error
	if err != nil {
		// Table might not exist yet, that's fine
		return nil
	}

	if count > 0 {
		// SQLite 3.25.0+ supports RENAME COLUMN
		// For older versions, we need to recreate the table
		err := db.Exec("ALTER TABLE routes RENAME COLUMN c_id_r TO cidr").Error
		if err != nil {
			// If RENAME COLUMN fails, try recreating the table
			return migrateRoutesTable(db)
		}
		// Also fix the unique constraint name
		db.Exec("DROP INDEX IF EXISTS uni_routes_c_id_r")
	}

	return nil
}

// migrateRoutesTable recreates routes table with correct column names (for old SQLite)
func migrateRoutesTable(db *gorm.DB) error {
	// Create new table with correct schema
	err := db.Exec(`
		CREATE TABLE IF NOT EXISTS routes_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			cidr TEXT NOT NULL UNIQUE,
			gateway TEXT,
			device TEXT,
			metric INTEGER,
			comment TEXT,
			enabled NUMERIC DEFAULT true,
			push_to_client NUMERIC DEFAULT true,
			apply_on_server NUMERIC DEFAULT false,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error
	if err != nil {
		return err
	}

	// Copy data from old table
	db.Exec(`
		INSERT INTO routes_new (id, cidr, gateway, device, metric, comment, enabled, push_to_client, apply_on_server, created_at, updated_at)
		SELECT id, c_id_r, gateway, device, metric, comment, enabled, push_to_client, apply_on_server, created_at, updated_at
		FROM routes
	`)

	// Drop old table and rename new one
	db.Exec("DROP TABLE routes")
	db.Exec("ALTER TABLE routes_new RENAME TO routes")

	return nil
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
