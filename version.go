package main

import (
	"fmt"
	"strconv"
	"strings"
)

// ProtocolVersion represents the stellar-mesh protocol version
type ProtocolVersion struct {
	Major int `json:"major"` // Breaking changes
	Minor int `json:"minor"` // New features, backwards compatible
	Patch int `json:"patch"` // Bug fixes, backwards compatible
}

// Current protocol version
var CurrentProtocolVersion = ProtocolVersion{
	Major: 1,
	Minor: 0,
	Patch: 0,
}

// ApplicationVersion represents the stellar-mesh application version
var ApplicationVersion = "1.0.0"

// String returns version as "major.minor.patch"
func (v ProtocolVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// ParseVersion parses a version string like "1.2.3"
func ParseVersion(s string) (ProtocolVersion, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return ProtocolVersion{}, fmt.Errorf("invalid version format: %s", s)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return ProtocolVersion{}, err
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return ProtocolVersion{}, err
	}

	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return ProtocolVersion{}, err
	}

	return ProtocolVersion{
		Major: major,
		Minor: minor,
		Patch: patch,
	}, nil
}

// IsCompatibleWith checks if this version can communicate with another version
// Compatible if:
// - Major versions match (breaking changes between majors)
// - This version >= other version (newer can talk to older)
func (v ProtocolVersion) IsCompatibleWith(other ProtocolVersion) bool {
	// Major version must match
	if v.Major != other.Major {
		return false
	}

	// Same major version = compatible
	// Newer minor versions must support older ones
	return true
}

// SupportsFeature checks if a version supports a specific feature
func (v ProtocolVersion) SupportsFeature(feature string) bool {
	// Feature flags based on version
	switch feature {
	case "attestations":
		// Attestations introduced in 1.0.0
		return v.Major >= 1

	case "multi_star_systems":
		// Multi-star introduced in 1.0.0
		return v.Major >= 1

	case "legacy_gossip":
		// Legacy gossip protocol (always supported for backwards compat)
		return true

	default:
		// Unknown features assumed not supported
		return false
	}
}

// FeatureNegotiation determines what features both nodes can use
type FeatureNegotiation struct {
	LocalVersion  ProtocolVersion `json:"local_version"`
	RemoteVersion ProtocolVersion `json:"remote_version"`
	Compatible    bool            `json:"compatible"`
	SharedFeatures []string       `json:"shared_features"`
}

// NegotiateFeatures determines what features can be used between two versions
func NegotiateFeatures(local, remote ProtocolVersion) *FeatureNegotiation {
	negotiation := &FeatureNegotiation{
		LocalVersion:  local,
		RemoteVersion: remote,
		Compatible:    local.IsCompatibleWith(remote),
		SharedFeatures: []string{},
	}

	if !negotiation.Compatible {
		return negotiation
	}

	// Check each feature
	features := []string{
		"attestations",
		"multi_star_systems",
		"legacy_gossip",
	}

	for _, feature := range features {
		if local.SupportsFeature(feature) && remote.SupportsFeature(feature) {
			negotiation.SharedFeatures = append(negotiation.SharedFeatures, feature)
		}
	}

	return negotiation
}

// ShouldSendAttestation returns true if peer supports attestations
func (fn *FeatureNegotiation) ShouldSendAttestation() bool {
	for _, f := range fn.SharedFeatures {
		if f == "attestations" {
			return true
		}
	}
	return false
}

// UseAttestations returns whether attestations should be used with this peer
func (fn *FeatureNegotiation) UseAttestations() bool {
	return fn.ShouldSendAttestation()
}

// VersionInfo contains version metadata sent with every message
type VersionInfo struct {
	Protocol    string `json:"protocol_version"`    // "1.0.0"
	Application string `json:"application_version"` // "1.0.0"
	Features    []string `json:"supported_features"` // ["attestations", "multi_star_systems", ...]
}

// GetVersionInfo returns current node's version info
func GetVersionInfo() VersionInfo {
	features := []string{}
	
	// List all features this version supports
	if CurrentProtocolVersion.SupportsFeature("attestations") {
		features = append(features, "attestations")
	}
	if CurrentProtocolVersion.SupportsFeature("multi_star_systems") {
		features = append(features, "multi_star_systems")
	}
	if CurrentProtocolVersion.SupportsFeature("legacy_gossip") {
		features = append(features, "legacy_gossip")
	}

	return VersionInfo{
		Protocol:    CurrentProtocolVersion.String(),
		Application: ApplicationVersion,
		Features:    features,
	}
}

// Comparison methods
func (v ProtocolVersion) GreaterThan(other ProtocolVersion) bool {
	if v.Major != other.Major {
		return v.Major > other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor > other.Minor
	}
	return v.Patch > other.Patch
}

func (v ProtocolVersion) GreaterThanOrEqual(other ProtocolVersion) bool {
	return v.GreaterThan(other) || v.Equals(other)
}

func (v ProtocolVersion) Equals(other ProtocolVersion) bool {
	return v.Major == other.Major && v.Minor == other.Minor && v.Patch == other.Patch
}
