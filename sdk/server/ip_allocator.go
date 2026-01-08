package server

import (
	"fmt"
	"net"
	"sync"
)

// IPAllocator manages IP address allocation for VPN clients.
type IPAllocator struct {
	mu       sync.Mutex
	subnet   *net.IPNet
	gateway  net.IP
	allocated map[string]net.IP // publicKey -> IP
	used     map[string]bool    // IP string -> used
}

// NewIPAllocator creates a new IP allocator for the given subnet.
// The first IP in the subnet is reserved for the gateway.
func NewIPAllocator(cidr string) (*IPAllocator, error) {
	_, subnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR: %w", err)
	}

	// Gateway is the first usable IP
	gateway := nextIP(subnet.IP)

	return &IPAllocator{
		subnet:    subnet,
		gateway:   gateway,
		allocated: make(map[string]net.IP),
		used:      map[string]bool{gateway.String(): true},
	}, nil
}

// Allocate allocates an IP address for the given public key.
// If the key already has an IP, it returns the existing allocation.
func (a *IPAllocator) Allocate(publicKey string) (net.IP, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if already allocated
	if ip, exists := a.allocated[publicKey]; exists {
		return ip, nil
	}

	// Find next available IP
	ip := nextIP(a.gateway)
	for a.subnet.Contains(ip) {
		if !a.used[ip.String()] {
			a.used[ip.String()] = true
			a.allocated[publicKey] = ip
			return ip, nil
		}
		ip = nextIP(ip)
	}

	return nil, fmt.Errorf("no available IP addresses in subnet")
}

// Release releases the IP address allocated to the given public key.
func (a *IPAllocator) Release(publicKey string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if ip, exists := a.allocated[publicKey]; exists {
		delete(a.used, ip.String())
		delete(a.allocated, publicKey)
	}
}

// GetAllocated returns the IP allocated to the given public key.
func (a *IPAllocator) GetAllocated(publicKey string) (net.IP, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	ip, exists := a.allocated[publicKey]
	return ip, exists
}

// GetGateway returns the gateway IP.
func (a *IPAllocator) GetGateway() net.IP {
	return a.gateway
}

// GetSubnet returns the subnet.
func (a *IPAllocator) GetSubnet() *net.IPNet {
	return a.subnet
}

// nextIP returns the next IP address.
func nextIP(ip net.IP) net.IP {
	next := make(net.IP, len(ip))
	copy(next, ip)

	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] != 0 {
			break
		}
	}

	return next
}
