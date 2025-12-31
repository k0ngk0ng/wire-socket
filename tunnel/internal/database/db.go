package database

import (
	"log"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB wraps gorm.DB
type DB struct {
	*gorm.DB
}

// AllocatedIP represents an IP allocation to a user
type AllocatedIP struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"column:user_id;not null;index" json:"user_id"`
	Username  string    `gorm:"column:username" json:"username"`
	IP        string    `gorm:"column:ip;not null;uniqueIndex" json:"ip"`
	PublicKey string    `gorm:"column:public_key" json:"public_key"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Route represents a route for VPN
type Route struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	CIDR          string    `gorm:"column:cidr;not null;unique" json:"cidr"`
	Gateway       string    `gorm:"column:gateway" json:"gateway,omitempty"`
	Device        string    `gorm:"column:device" json:"device,omitempty"`
	Metric        int       `gorm:"column:metric" json:"metric,omitempty"`
	Comment       string    `gorm:"column:comment" json:"comment"`
	Enabled       bool      `gorm:"column:enabled;default:true" json:"enabled"`
	PushToClient  bool      `gorm:"column:push_to_client;default:true" json:"push_to_client"`
	ApplyOnServer bool      `gorm:"column:apply_on_server;default:false" json:"apply_on_server"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// NATType represents the type of NAT rule
type NATType string

const (
	NATTypeMasquerade NATType = "masquerade"
	NATTypeSNAT       NATType = "snat"
	NATTypeDNAT       NATType = "dnat"
	NATTypeTCPMSS     NATType = "tcpmss"
)

// NATRule represents a NAT rule
type NATRule struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Type          NATType   `gorm:"column:type;not null" json:"type"`
	Interface     string    `gorm:"column:interface" json:"interface"`
	Source        string    `gorm:"column:source" json:"source,omitempty"`
	Destination   string    `gorm:"column:destination" json:"destination,omitempty"`
	ToSource      string    `gorm:"column:to_source" json:"to_source,omitempty"`
	ToDestination string    `gorm:"column:to_destination" json:"to_destination,omitempty"`
	Protocol      string    `gorm:"column:protocol" json:"protocol,omitempty"`
	Port          int       `gorm:"column:port" json:"port,omitempty"`
	MSS           int       `gorm:"column:mss" json:"mss,omitempty"`
	Enabled       bool      `gorm:"column:enabled;default:true" json:"enabled"`
	Comment       string    `gorm:"column:comment" json:"comment"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// NewDB creates a new database connection
func NewDB(dbPath string) (*DB, error) {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}

	return &DB{DB: db}, nil
}

// AutoMigrate creates/updates database tables
func (db *DB) AutoMigrate() error {
	if err := db.DB.AutoMigrate(
		&AllocatedIP{},
		&Route{},
		&NATRule{},
	); err != nil {
		return err
	}

	// Migrate column names if needed (CIDR -> cidr, etc.)
	db.migrateColumnNames()
	return nil
}

// migrateColumnNames fixes column naming issues from GORM
func (db *DB) migrateColumnNames() {
	// Check if old column exists and migrate
	var count int64
	db.Raw("SELECT COUNT(*) FROM pragma_table_info('routes') WHERE name = 'c_id_r'").Scan(&count)
	if count > 0 {
		log.Println("Migrating routes table: c_id_r -> cidr")
		db.Exec("ALTER TABLE routes RENAME COLUMN c_id_r TO cidr")
	}

	db.Raw("SELECT COUNT(*) FROM pragma_table_info('nat_rules') WHERE name = 'm_s_s'").Scan(&count)
	if count > 0 {
		log.Println("Migrating nat_rules table: m_s_s -> mss")
		db.Exec("ALTER TABLE nat_rules RENAME COLUMN m_s_s TO mss")
	}
}

// GetEnabledRoutes returns all enabled routes for clients
func (db *DB) GetEnabledRoutes() ([]string, error) {
	var routes []Route
	if err := db.Where("enabled = ? AND push_to_client = ?", true, true).Find(&routes).Error; err != nil {
		return nil, err
	}

	cidrs := make([]string, len(routes))
	for i, r := range routes {
		cidrs[i] = r.CIDR
	}
	return cidrs, nil
}

// GetOrCreateIP allocates an IP for a user
func (db *DB) GetOrCreateIP(userID uint, username string, subnet string) (*AllocatedIP, error) {
	var existing AllocatedIP
	if err := db.Where("user_id = ?", userID).First(&existing).Error; err == nil {
		return &existing, nil
	}

	// Allocate new IP from subnet
	ip, err := db.allocateNextIP(subnet)
	if err != nil {
		return nil, err
	}

	allocated := AllocatedIP{
		UserID:   userID,
		Username: username,
		IP:       ip,
	}

	if err := db.Create(&allocated).Error; err != nil {
		return nil, err
	}

	return &allocated, nil
}

// allocateNextIP finds the next available IP in the subnet
func (db *DB) allocateNextIP(subnet string) (string, error) {
	// Parse subnet to get base IP
	// For simplicity, assume /24 subnet like 10.0.0.0/24
	// Skip .0 (network), .1 (gateway), start from .2

	var allocations []AllocatedIP
	if err := db.Find(&allocations).Error; err != nil {
		return "", err
	}

	// Extract base from subnet (e.g., "10.0.0" from "10.0.0.0/24")
	base := subnet[:len(subnet)-5] // Remove ".0/24"

	used := make(map[string]bool)
	for _, a := range allocations {
		used[a.IP] = true
	}

	// Find first available IP (start from .2)
	for i := 2; i < 255; i++ {
		ip := base + "." + string(rune('0'+i/100)) + string(rune('0'+(i/10)%10)) + string(rune('0'+i%10))
		// Simpler approach
		ip = base + "." + itoa(i)
		if !used[ip] {
			return ip, nil
		}
	}

	return "", gorm.ErrRecordNotFound
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	result := ""
	for i > 0 {
		result = string(rune('0'+i%10)) + result
		i /= 10
	}
	return result
}

// UpdatePublicKey updates the public key for an allocated IP
func (db *DB) UpdatePublicKey(userID uint, publicKey string) error {
	return db.Model(&AllocatedIP{}).Where("user_id = ?", userID).Update("public_key", publicKey).Error
}
