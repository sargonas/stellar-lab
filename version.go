package main

import (
	"fmt"
	"strconv"
	"strings"
)

// BuildVersion is set at build time via ldflags
// Example: go build -ldflags "-X main.BuildVersion=1.2.3"
var BuildVersion = "dev"

// CurrentProtocolVersion is parsed from BuildVersion at init
var CurrentProtocolVersion ProtocolVersion

func init() {
	v, err := ParseVersion(BuildVersion)
	if err != nil {
		// Fallback for dev builds - obviously not a release
		CurrentProtocolVersion = ProtocolVersion{Major: 0, Minor: 0, Patch: 0}
	} else {
		CurrentProtocolVersion = v
	}
}

// ProtocolVersion represents a semantic version
type ProtocolVersion struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
	Patch int `json:"patch"`
}

// String returns the version as a string (e.g., "1.0.0")
func (v ProtocolVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// ParseVersion parses a version string into a ProtocolVersion
func ParseVersion(s string) (ProtocolVersion, error) {
	// Strip leading 'v' if present
	s = strings.TrimPrefix(s, "v")
	
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