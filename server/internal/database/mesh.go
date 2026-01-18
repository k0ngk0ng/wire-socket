package database

import "time"

// MeshNode represents a node in the Mesh network
type MeshNode struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	Name        string     `gorm:"column:name;unique;not null" json:"name"`          // Node name (unique in Mesh)
	PublicKey   string     `gorm:"column:public_key;unique;not null" json:"public_key"` // WireGuard public key
	PrivateKey  string     `gorm:"column:private_key" json:"-"`                       // WireGuard private key (only for local node)
	MeshIP      string     `gorm:"column:mesh_ip;unique;not null" json:"mesh_ip"`     // Mesh internal IP (10.254.0.x)
	TunnelURL   string     `gorm:"column:tunnel_url" json:"tunnel_url,omitempty"`     // WSS tunnel address (wss://host:port/path)
	APIEndpoint string     `gorm:"column:api_endpoint" json:"api_endpoint,omitempty"` // API address (https://host:port)
	IsLocal     bool       `gorm:"column:is_local;default:false" json:"is_local"`     // Is this the local node
	IsOnline    bool       `gorm:"column:is_online;default:false" json:"is_online"`   // Online status
	LastSeen    *time.Time `gorm:"column:last_seen" json:"last_seen,omitempty"`       // Last online time
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`

	// Loaded from ExitRoutes table
	ExitRoutes []ExitRoute `gorm:"foreignKey:NodeID" json:"exit_routes,omitempty"`
}

// ExitRoute declares a network that a node can access
type ExitRoute struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	NodeID    uint      `gorm:"column:node_id;not null;index" json:"node_id"` // Owning node
	CIDR      string    `gorm:"column:cidr;not null" json:"cidr"`             // Accessible network segment
	Comment   string    `gorm:"column:comment" json:"comment,omitempty"`      // Description
	Enabled   bool      `gorm:"column:enabled;default:true" json:"enabled"`
	Priority  int       `gorm:"column:priority;default:100" json:"priority"` // Lower = higher priority
	CreatedAt time.Time `json:"created_at"`

	Node MeshNode `gorm:"foreignKey:NodeID" json:"-"`
}

// MeshNodeRole defines the role of a mesh node
type MeshNodeRole string

const (
	MeshRoleGateway MeshNodeRole = "gateway" // Entry node, accepts client connections
	MeshRoleExit    MeshNodeRole = "exit"    // Exit node, only accepts mesh connections
	MeshRoleBoth    MeshNodeRole = "both"    // Dual role
)

// TableName specifies the table name for MeshNode
func (MeshNode) TableName() string {
	return "mesh_nodes"
}

// TableName specifies the table name for ExitRoute
func (ExitRoute) TableName() string {
	return "exit_routes"
}
