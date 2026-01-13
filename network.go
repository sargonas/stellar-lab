package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/libp2p/go-nat"
)

// === Hardware Identification ===

// HardwareIdentifier contains various hardware identifiers
type HardwareIdentifier struct {
	MACAddress   string
	Hostname     string
	MachineID    string
	CombinedHash string
}

// GetHardwareID attempts to get a stable hardware identifier
func GetHardwareID() (*HardwareIdentifier, error) {
	hwid := &HardwareIdentifier{}

	// Get MAC address
	interfaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range interfaces {
			// Skip loopback and virtual interfaces
			if iface.Flags&net.FlagLoopback != 0 || strings.HasPrefix(iface.Name, "veth") {
				continue
			}
			if iface.HardwareAddr != nil && len(iface.HardwareAddr) > 0 {
				hwid.MACAddress = iface.HardwareAddr.String()
				break
			}
		}
	}

	// Get hostname
	hostname, err := os.Hostname()
	if err == nil {
		hwid.Hostname = hostname
	}

	// Try to get machine-id (Linux)
	if runtime.GOOS == "linux" {
		if data, err := os.ReadFile("/etc/machine-id"); err == nil {
			hwid.MachineID = strings.TrimSpace(string(data))
		} else if data, err := os.ReadFile("/var/lib/dbus/machine-id"); err == nil {
			hwid.MachineID = strings.TrimSpace(string(data))
		}
	}

	// Create combined hash
	combined := fmt.Sprintf("%s|%s|%s", hwid.MACAddress, hwid.Hostname, hwid.MachineID)
	hash := sha256.Sum256([]byte(combined))
	hwid.CombinedHash = fmt.Sprintf("%x", hash[:8])

	return hwid, nil
}

// GenerateSemiDeterministicUUID creates a UUID based on hardware ID and optional seed
// If seed is provided, the same hardware + seed will always generate the same UUID
// This allows for deterministic regeneration while still being unique per system
func GenerateSemiDeterministicUUID(seed string) (uuid.UUID, error) {
	hwid, err := GetHardwareID()
	if err != nil {
		// Fall back to random UUID if we can't get hardware ID
		return uuid.New(), nil
	}

	// Combine hardware ID with user seed
	combined := fmt.Sprintf("%s|%s|%s|%s",
		hwid.MACAddress,
		hwid.Hostname,
		hwid.MachineID,
		seed)

	// Hash to create deterministic UUID
	hash := sha256.Sum256([]byte(combined))

	// Create UUID from hash (Version 5 style)
	var u uuid.UUID
	copy(u[:], hash[:16])

	// Set version (5) and variant bits
	u[6] = (u[6] & 0x0f) | 0x50 // Version 5
	u[8] = (u[8] & 0x3f) | 0x80 // Variant

	return u, nil
}

// GenerateRandomUUID creates a completely random UUID (for comparison)
func GenerateRandomUUID() uuid.UUID {
	return uuid.New()
}

// GetHardwareFingerprint returns a short fingerprint of the hardware
func GetHardwareFingerprint() string {
	hwid, err := GetHardwareID()
	if err != nil {
		return "unknown"
	}
	return hwid.CombinedHash
}

// === NAT Traversal ===

// NATTraversal handles automatic port forwarding via UPnP or NAT-PMP
type NATTraversal struct {
	nat      nat.NAT
	port     int
	stopChan chan struct{}
}

// NATConfig holds configuration for NAT traversal
type NATConfig struct {
	InternalPort  int
	ExternalPort  int
	Description   string
	LeaseDuration time.Duration
}

// NewNATTraversal creates a new NAT traversal handler
func NewNATTraversal() *NATTraversal {
	return &NATTraversal{
		stopChan: make(chan struct{}),
	}
}

// Setup attempts to configure port forwarding using UPnP or NAT-PMP
// Returns the external address (ip:port) if successful
func (n *NATTraversal) Setup(config NATConfig) (string, error) {
	if config.ExternalPort == 0 {
		config.ExternalPort = config.InternalPort
	}
	if config.Description == "" {
		config.Description = "Stellar Lab P2P"
	}
	if config.LeaseDuration == 0 {
		config.LeaseDuration = 2 * time.Hour
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Discover NAT gateway (tries UPnP then NAT-PMP automatically)
	gateway, err := nat.DiscoverGateway(ctx)
	if err != nil {
		return "", fmt.Errorf("no NAT gateway found: %w", err)
	}

	n.nat = gateway
	n.port = config.ExternalPort

	// Get external IP
	extIP, err := gateway.GetExternalAddress()
	if err != nil {
		return "", fmt.Errorf("failed to get external address: %w", err)
	}

	// Add port mapping
	_, err = gateway.AddPortMapping(ctx, "tcp", config.ExternalPort, config.Description, config.LeaseDuration)
	if err != nil {
		return "", fmt.Errorf("failed to add port mapping: %w", err)
	}

	// Start renewal loop (renew at half the lease duration)
	go n.renewLoop(config.Description, config.LeaseDuration)

	return fmt.Sprintf("%s:%d", extIP.String(), config.ExternalPort), nil
}

// renewLoop periodically renews the port mapping before it expires
func (n *NATTraversal) renewLoop(description string, leaseDuration time.Duration) {
	// Renew at half the lease duration to ensure we never expire
	ticker := time.NewTicker(leaseDuration / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_, err := n.nat.AddPortMapping(ctx, "tcp", n.port, description, leaseDuration)
			cancel()
			if err != nil {
				log.Printf("UPnP/NAT-PMP: failed to renew port mapping: %v", err)
			}
		case <-n.stopChan:
			return
		}
	}
}

// Close removes the port mapping and stops the renewal loop
func (n *NATTraversal) Close() {
	close(n.stopChan)
	if n.nat != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := n.nat.DeletePortMapping(ctx, "tcp", n.port); err != nil {
			log.Printf("UPnP/NAT-PMP: failed to remove port mapping: %v", err)
		}
	}
}

// GetProtocol returns the NAT traversal method being used ("UPnP" or "NAT-PMP")
func (n *NATTraversal) GetProtocol() string {
	if n.nat != nil {
		return n.nat.Type()
	}
	return "unknown"
}
