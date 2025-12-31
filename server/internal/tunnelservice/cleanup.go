package tunnelservice

import (
	"fmt"
	"sync"
	"time"
	"wire-socket-server/internal/database"
	"wire-socket-server/internal/wireguard"
)

// PeerCleanup handles periodic cleanup of inactive WireGuard peers
type PeerCleanup struct {
	db           *database.TunnelDB
	wgManager    *wireguard.Manager
	timeout      time.Duration
	interval     time.Duration
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

// CleanupConfig configures the peer cleanup service
type CleanupConfig struct {
	// Timeout is how long a peer can be inactive before removal (default: 3 minutes)
	Timeout time.Duration
	// Interval is how often to check for inactive peers (default: 30 seconds)
	Interval time.Duration
}

// DefaultCleanupConfig returns the default cleanup configuration
func DefaultCleanupConfig() CleanupConfig {
	return CleanupConfig{
		Timeout:  3 * time.Minute,
		Interval: 30 * time.Second,
	}
}

// NewPeerCleanup creates a new peer cleanup service
func NewPeerCleanup(db *database.TunnelDB, wgManager *wireguard.Manager, config CleanupConfig) *PeerCleanup {
	if config.Timeout == 0 {
		config.Timeout = 3 * time.Minute
	}
	if config.Interval == 0 {
		config.Interval = 30 * time.Second
	}

	return &PeerCleanup{
		db:        db,
		wgManager: wgManager,
		timeout:   config.Timeout,
		interval:  config.Interval,
		stopCh:    make(chan struct{}),
	}
}

// Start begins the periodic cleanup process
func (c *PeerCleanup) Start() {
	c.wg.Add(1)
	go c.run()
	fmt.Printf("Peer cleanup started (timeout: %v, interval: %v)\n", c.timeout, c.interval)
}

// Stop stops the cleanup process
func (c *PeerCleanup) Stop() {
	close(c.stopCh)
	c.wg.Wait()
	fmt.Println("Peer cleanup stopped")
}

// run is the main cleanup loop
func (c *PeerCleanup) run() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.cleanup()
		}
	}
}

// cleanup removes inactive peers
func (c *PeerCleanup) cleanup() {
	stats, err := c.wgManager.GetPeerStats()
	if err != nil {
		fmt.Printf("Cleanup: failed to get peer stats: %v\n", err)
		return
	}

	now := time.Now()
	for _, peer := range stats {
		// Skip peers that have never had a handshake (just added, waiting for connection)
		if peer.LastHandshake.IsZero() {
			continue
		}

		// Check if peer is inactive
		inactive := now.Sub(peer.LastHandshake)
		if inactive > c.timeout {
			fmt.Printf("Cleanup: removing inactive peer %s (last handshake: %v ago)\n",
				peer.PublicKey[:8]+"...", inactive.Round(time.Second))

			// Remove from WireGuard
			if err := c.wgManager.RemovePeer(peer.PublicKey); err != nil {
				fmt.Printf("Cleanup: failed to remove peer %s: %v\n", peer.PublicKey[:8]+"...", err)
				continue
			}

			// Mark as disconnected in database
			if err := c.db.MarkPeerDisconnected(peer.PublicKey); err != nil {
				fmt.Printf("Cleanup: failed to update DB for peer %s: %v\n", peer.PublicKey[:8]+"...", err)
			}
		}
	}
}
