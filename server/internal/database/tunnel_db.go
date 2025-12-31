package database

import (
	"log"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ============ Tunnel Service Models ============

// TunnelAllocatedIP represents an IP allocation to a user on a tunnel node
type TunnelAllocatedIP struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"column:user_id;not null;index" json:"user_id"`
	Username  string    `gorm:"column:username" json:"username"`
	IP        string    `gorm:"column:ip;not null;uniqueIndex" json:"ip"`
	PublicKey string    `gorm:"column:public_key" json:"public_key"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName overrides the table name for TunnelAllocatedIP
func (TunnelAllocatedIP) TableName() string {
	return "allocated_ips"
}

// TunnelRoute represents a route for VPN on a tunnel node
type TunnelRoute struct {
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

// TableName overrides the table name for TunnelRoute
func (TunnelRoute) TableName() string {
	return "routes"
}

// TunnelNATType represents the type of NAT rule
type TunnelNATType string

const (
	TunnelNATTypeMasquerade TunnelNATType = "masquerade"
	TunnelNATTypeSNAT       TunnelNATType = "snat"
	TunnelNATTypeDNAT       TunnelNATType = "dnat"
	TunnelNATTypeTCPMSS     TunnelNATType = "tcpmss"
)

// TunnelNATRule represents a NAT rule on a tunnel node
type TunnelNATRule struct {
	ID            uint          `gorm:"primaryKey" json:"id"`
	Type          TunnelNATType `gorm:"column:type;not null" json:"type"`
	Interface     string        `gorm:"column:interface" json:"interface"`
	Source        string        `gorm:"column:source" json:"source,omitempty"`
	Destination   string        `gorm:"column:destination" json:"destination,omitempty"`
	ToSource      string        `gorm:"column:to_source" json:"to_source,omitempty"`
	ToDestination string        `gorm:"column:to_destination" json:"to_destination,omitempty"`
	Protocol      string        `gorm:"column:protocol" json:"protocol,omitempty"`
	Port          int           `gorm:"column:port" json:"port,omitempty"`
	MSS           int           `gorm:"column:mss" json:"mss,omitempty"`
	Enabled       bool          `gorm:"column:enabled;default:true" json:"enabled"`
	Comment       string        `gorm:"column:comment" json:"comment"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// TableName overrides the table name for TunnelNATRule
func (TunnelNATRule) TableName() string {
	return "nat_rules"
}

// TunnelDB wraps gorm.DB for tunnel service
type TunnelDB struct {
	*gorm.DB
}

// NewTunnelDB creates a new tunnel service database connection
func NewTunnelDB(dbPath string) (*TunnelDB, error) {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}

	return &TunnelDB{DB: db}, nil
}

// AutoMigrate creates/updates tunnel service database tables
func (db *TunnelDB) AutoMigrate() error {
	if err := db.DB.AutoMigrate(
		&TunnelAllocatedIP{},
		&TunnelRoute{},
		&TunnelNATRule{},
	); err != nil {
		return err
	}

	// Migrate column names if needed (CIDR -> cidr, etc.)
	db.migrateColumnNames()
	return nil
}

// migrateColumnNames fixes column naming issues from GORM
func (db *TunnelDB) migrateColumnNames() {
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
func (db *TunnelDB) GetEnabledRoutes() ([]string, error) {
	var routes []TunnelRoute
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
func (db *TunnelDB) GetOrCreateIP(userID uint, username string, subnet string) (*TunnelAllocatedIP, error) {
	var existing TunnelAllocatedIP
	if err := db.Where("user_id = ?", userID).First(&existing).Error; err == nil {
		return &existing, nil
	}

	// Allocate new IP from subnet
	ip, err := db.allocateNextIP(subnet)
	if err != nil {
		return nil, err
	}

	allocated := TunnelAllocatedIP{
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
func (db *TunnelDB) allocateNextIP(subnet string) (string, error) {
	// Parse subnet to get base IP
	// For simplicity, assume /24 subnet like 10.0.0.0/24
	// Skip .0 (network), .1 (gateway), start from .2

	var allocations []TunnelAllocatedIP
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
		ip := base + "." + itoa(i)
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
func (db *TunnelDB) UpdatePublicKey(userID uint, publicKey string) error {
	return db.Model(&TunnelAllocatedIP{}).Where("user_id = ?", userID).Update("public_key", publicKey).Error
}

// MarkPeerDisconnected clears the public key for a disconnected peer
func (db *TunnelDB) MarkPeerDisconnected(publicKey string) error {
	return db.Model(&TunnelAllocatedIP{}).Where("public_key = ?", publicKey).Update("public_key", "").Error
}
