package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/libp2p/go-nat"
)

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