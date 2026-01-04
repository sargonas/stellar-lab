package main

import (
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"runtime"
	"strings"

	"github.com/google/uuid"
)

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
		if data, err := ioutil.ReadFile("/etc/machine-id"); err == nil {
			hwid.MachineID = strings.TrimSpace(string(data))
		} else if data, err := ioutil.ReadFile("/var/lib/dbus/machine-id"); err == nil {
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
