package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Protocol version for DHT-based stellar-lab v1.0.0
const (
	ProtocolMajor = 1
	ProtocolMinor = 2
	ProtocolPatch = 1
)

var CurrentProtocolVersion = ProtocolVersion{
	Major: ProtocolMajor,
	Minor: ProtocolMinor,
	Patch: ProtocolPatch,
}

// ProtocolVersion represents a semantic version
type ProtocolVersion struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
	Patch int `json:"patch"`
}

// String returns the version as a string (e.g., "2.0.0")
func (v ProtocolVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// ParseVersion parses a version string into a ProtocolVersion
func ParseVersion(s string) (ProtocolVersion, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return ProtocolVersion{}, fmt.Errorf("invalid version format: %s", s)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return ProtocolVersion{}, fmt.Errorf("invalid major version: %s", parts[0])
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return ProtocolVersion{}, fmt.Errorf("invalid minor version: %s", parts[1])
	}

	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return ProtocolVersion{}, fmt.Errorf("invalid patch version: %s", parts[2])
	}

	return ProtocolVersion{Major: major, Minor: minor, Patch: patch}, nil
}

// IsCompatibleWith checks if this version is compatible with another
// Major version must match for compatibility
func (v ProtocolVersion) IsCompatibleWith(other ProtocolVersion) bool {
	return v.Major == other.Major
}

// IsNewerThan returns true if this version is newer than other
func (v ProtocolVersion) IsNewerThan(other ProtocolVersion) bool {
	if v.Major != other.Major {
		return v.Major > other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor > other.Minor
	}
	return v.Patch > other.Patch
}

// VersionInfo contains version information for protocol messages
type VersionInfo struct {
	Protocol string `json:"protocol"` // Protocol version (e.g., "2.0.0")
	Software string `json:"software"` // Software identifier
}

// GetVersionInfo returns the current version info for messages
func GetVersionInfo() VersionInfo {
	return VersionInfo{
		Protocol: CurrentProtocolVersion.String(),
		Software: "stellar-lab",
	}
}
