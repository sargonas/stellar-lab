package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type Storage struct {
	db *sql.DB
}

// NewStorage initializes SQLite database and creates tables
func NewStorage(dbPath string) (*Storage, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	storage := &Storage{db: db}
	if err := storage.createTables(); err != nil {
		return nil, err
	}

	return storage, nil
}

func (s *Storage) createTables() error {
	schema := `
	CREATE TABLE IF NOT EXISTS system (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		x REAL NOT NULL,
		y REAL NOT NULL,
		z REAL NOT NULL,
		-- Primary star
		primary_class TEXT NOT NULL,
		primary_description TEXT NOT NULL,
		primary_color TEXT NOT NULL,
		primary_temperature INTEGER NOT NULL,
		primary_luminosity REAL NOT NULL,
		-- Secondary star (nullable for single-star systems)
		secondary_class TEXT,
		secondary_description TEXT,
		secondary_color TEXT,
		secondary_temperature INTEGER,
		secondary_luminosity REAL,
		-- Tertiary star (nullable)
		tertiary_class TEXT,
		tertiary_description TEXT,
		tertiary_color TEXT,
		tertiary_temperature INTEGER,
		tertiary_luminosity REAL,
		-- Multi-star metadata
		is_binary INTEGER NOT NULL DEFAULT 0,
		is_trinary INTEGER NOT NULL DEFAULT 0,
		star_count INTEGER NOT NULL DEFAULT 1,
		-- System metadata
		created_at INTEGER NOT NULL,
		last_seen_at INTEGER NOT NULL,
		address TEXT NOT NULL,
		-- Cryptographic identity
		public_key TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS peers (
		system_id TEXT PRIMARY KEY,
		address TEXT NOT NULL,
		last_seen_at INTEGER NOT NULL,
		total_messages INTEGER NOT NULL DEFAULT 0,
		public_key TEXT,
		FOREIGN KEY (system_id) REFERENCES system(id)
	);

	CREATE TABLE IF NOT EXISTS attestations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		from_system_id TEXT NOT NULL,
		to_system_id TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		message_type TEXT NOT NULL,
		signature TEXT NOT NULL,
		public_key TEXT NOT NULL,
		verified INTEGER DEFAULT 0,
		created_at INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_peers_last_seen ON peers(last_seen_at);
	CREATE INDEX IF NOT EXISTS idx_system_coords ON system(x, y, z);
	CREATE INDEX IF NOT EXISTS idx_system_primary_class ON system(primary_class);
	CREATE INDEX IF NOT EXISTS idx_system_star_count ON system(star_count);
	CREATE INDEX IF NOT EXISTS idx_attestations_from ON attestations(from_system_id);
	CREATE INDEX IF NOT EXISTS idx_attestations_to ON attestations(to_system_id);
	CREATE INDEX IF NOT EXISTS idx_attestations_timestamp ON attestations(timestamp);
	CREATE INDEX IF NOT EXISTS idx_attestations_verified ON attestations(verified);
	`

	_, err := s.db.Exec(schema)
	return err
}

// SaveSystem persists the local system info
func (s *Storage) SaveSystem(sys *System) error {
	// Prepare nullable star values
	var secondaryClass, secondaryDesc, secondaryColor *string
	var secondaryTemp *int
	var secondaryLum *float64
	
	var tertiaryClass, tertiaryDesc, tertiaryColor *string
	var tertiaryTemp *int
	var tertiaryLum *float64
	
	if sys.Stars.Secondary != nil {
		secondaryClass = &sys.Stars.Secondary.Class
		secondaryDesc = &sys.Stars.Secondary.Description
		secondaryColor = &sys.Stars.Secondary.Color
		secondaryTemp = &sys.Stars.Secondary.Temperature
		secondaryLum = &sys.Stars.Secondary.Luminosity
	}
	
	if sys.Stars.Tertiary != nil {
		tertiaryClass = &sys.Stars.Tertiary.Class
		tertiaryDesc = &sys.Stars.Tertiary.Description
		tertiaryColor = &sys.Stars.Tertiary.Color
		tertiaryTemp = &sys.Stars.Tertiary.Temperature
		tertiaryLum = &sys.Stars.Tertiary.Luminosity
	}
	
	isBinary := 0
	if sys.Stars.IsBinary {
		isBinary = 1
	}
	isTrinary := 0
	if sys.Stars.IsTrinary {
		isTrinary = 1
	}
	
	// Encode public key
	publicKey := ""
	if sys.Keys != nil {
		publicKey = base64.StdEncoding.EncodeToString(sys.Keys.PublicKey)
	}
	
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO system (
			id, name, x, y, z,
			primary_class, primary_description, primary_color, primary_temperature, primary_luminosity,
			secondary_class, secondary_description, secondary_color, secondary_temperature, secondary_luminosity,
			tertiary_class, tertiary_description, tertiary_color, tertiary_temperature, tertiary_luminosity,
			is_binary, is_trinary, star_count,
			created_at, last_seen_at, address, public_key
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, sys.ID.String(), sys.Name, sys.X, sys.Y, sys.Z,
		sys.Stars.Primary.Class, sys.Stars.Primary.Description, sys.Stars.Primary.Color,
		sys.Stars.Primary.Temperature, sys.Stars.Primary.Luminosity,
		secondaryClass, secondaryDesc, secondaryColor, secondaryTemp, secondaryLum,
		tertiaryClass, tertiaryDesc, tertiaryColor, tertiaryTemp, tertiaryLum,
		isBinary, isTrinary, sys.Stars.Count,
		sys.CreatedAt.Unix(), sys.LastSeenAt.Unix(), sys.Address, publicKey)
	return err
}

// LoadSystem retrieves the local system info
func (s *Storage) LoadSystem() (*System, error) {
	var sys System
	var idStr string
	var createdAt, lastSeenAt int64
	var isBinary, isTrinary, starCount int
	
	// Nullable fields for secondary/tertiary stars
	var secondaryClass, secondaryDesc, secondaryColor sql.NullString
	var secondaryTemp sql.NullInt64
	var secondaryLum sql.NullFloat64
	
	var tertiaryClass, tertiaryDesc, tertiaryColor sql.NullString
	var tertiaryTemp sql.NullInt64
	var tertiaryLum sql.NullFloat64

	err := s.db.QueryRow(`
		SELECT id, name, x, y, z,
			primary_class, primary_description, primary_color, primary_temperature, primary_luminosity,
			secondary_class, secondary_description, secondary_color, secondary_temperature, secondary_luminosity,
			tertiary_class, tertiary_description, tertiary_color, tertiary_temperature, tertiary_luminosity,
			is_binary, is_trinary, star_count,
			created_at, last_seen_at, address
		FROM system LIMIT 1
	`).Scan(&idStr, &sys.Name, &sys.X, &sys.Y, &sys.Z,
		&sys.Stars.Primary.Class, &sys.Stars.Primary.Description, &sys.Stars.Primary.Color,
		&sys.Stars.Primary.Temperature, &sys.Stars.Primary.Luminosity,
		&secondaryClass, &secondaryDesc, &secondaryColor, &secondaryTemp, &secondaryLum,
		&tertiaryClass, &tertiaryDesc, &tertiaryColor, &tertiaryTemp, &tertiaryLum,
		&isBinary, &isTrinary, &starCount,
		&createdAt, &lastSeenAt, &sys.Address)

	if err != nil {
		return nil, err
	}

	sys.ID = uuid.MustParse(idStr)
	sys.CreatedAt = time.Unix(createdAt, 0)
	sys.LastSeenAt = time.Unix(lastSeenAt, 0)
	
	sys.Stars.IsBinary = isBinary == 1
	sys.Stars.IsTrinary = isTrinary == 1
	sys.Stars.Count = starCount
	
	// Load secondary star if present
	if secondaryClass.Valid {
		sys.Stars.Secondary = &StarType{
			Class:       secondaryClass.String,
			Description: secondaryDesc.String,
			Color:       secondaryColor.String,
			Temperature: int(secondaryTemp.Int64),
			Luminosity:  secondaryLum.Float64,
		}
	}
	
	// Load tertiary star if present
	if tertiaryClass.Valid {
		sys.Stars.Tertiary = &StarType{
			Class:       tertiaryClass.String,
			Description: tertiaryDesc.String,
			Color:       tertiaryColor.String,
			Temperature: int(tertiaryTemp.Int64),
			Luminosity:  tertiaryLum.Float64,
		}
	}

	return &sys, nil
}

// SavePeer adds or updates a peer
func (s *Storage) SavePeer(peer *Peer) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO peers (system_id, address, last_seen_at)
		VALUES (?, ?, ?)
	`, peer.SystemID.String(), peer.Address, peer.LastSeenAt.Unix())
	return err
}

// GetPeers retrieves all known peers
func (s *Storage) GetPeers() ([]*Peer, error) {
	rows, err := s.db.Query(`
		SELECT system_id, address, last_seen_at
		FROM peers
		ORDER BY last_seen_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var peers []*Peer
	for rows.Next() {
		var peer Peer
		var idStr string
		var lastSeenAt int64

		if err := rows.Scan(&idStr, &peer.Address, &lastSeenAt); err != nil {
			continue
		}

		peer.SystemID = uuid.MustParse(idStr)
		peer.LastSeenAt = time.Unix(lastSeenAt, 0)
		peers = append(peers, &peer)
	}

	return peers, nil
}

// PruneDeadPeers removes peers not seen within the threshold
func (s *Storage) PruneDeadPeers(threshold time.Duration) error {
	cutoff := time.Now().Add(-threshold).Unix()
	_, err := s.db.Exec(`DELETE FROM peers WHERE last_seen_at < ?`, cutoff)
	return err
}

// GetPeerCount returns the number of known peers
func (s *Storage) GetPeerCount() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM peers`).Scan(&count)
	return count, err
}

// Close closes the database connection
func (s *Storage) Close() error {
	return s.db.Close()
}

// GetStats returns basic statistics about the stored data
func (s *Storage) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	var peerCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM peers`).Scan(&peerCount); err != nil {
		return nil, err
	}
	stats["peer_count"] = peerCount

	return stats, nil
}

// SaveAttestation stores a cryptographically signed attestation
func (s *Storage) SaveAttestation(attestation *Attestation) error {
	verified := 0
	if attestation.Verify() {
		verified = 1
	}
	
	_, err := s.db.Exec(`
		INSERT INTO attestations (
			from_system_id, to_system_id, timestamp, message_type,
			signature, public_key, verified, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, attestation.FromSystemID.String(), attestation.ToSystemID.String(),
		attestation.Timestamp, attestation.MessageType,
		attestation.Signature, attestation.PublicKey, verified, time.Now().Unix())
	
	return err
}

// GetAttestations retrieves all attestations for a system
func (s *Storage) GetAttestations(systemID uuid.UUID) ([]*Attestation, error) {
	rows, err := s.db.Query(`
		SELECT from_system_id, to_system_id, timestamp, message_type, signature, public_key
		FROM attestations
		WHERE from_system_id = ? OR to_system_id = ?
		ORDER BY timestamp ASC
	`, systemID.String(), systemID.String())
	
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var attestations []*Attestation
	for rows.Next() {
		var att Attestation
		var fromID, toID string
		
		err := rows.Scan(&fromID, &toID, &att.Timestamp, &att.MessageType, &att.Signature, &att.PublicKey)
		if err != nil {
			continue
		}
		
		att.FromSystemID = uuid.MustParse(fromID)
		att.ToSystemID = uuid.MustParse(toID)
		attestations = append(attestations, &att)
	}
	
	return attestations, nil
}

// GetAttestationCount returns the count of verified attestations
func (s *Storage) GetAttestationCount(systemID uuid.UUID) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM attestations
		WHERE (from_system_id = ? OR to_system_id = ?)
		AND verified = 1
	`, systemID.String(), systemID.String()).Scan(&count)
	
	return count, err
}

// IncrementPeerMessageCount tracks communication frequency
func (s *Storage) IncrementPeerMessageCount(peerID uuid.UUID) error {
	_, err := s.db.Exec(`
		UPDATE peers SET total_messages = total_messages + 1 WHERE system_id = ?
	`, peerID.String())
	return err
}
