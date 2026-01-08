package main

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"log"
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

	// Enable WAL mode for better concurrent access
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA busy_timeout=5000")

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
		peer_address TEXT NOT NULL,
		sponsor_id TEXT,
		-- Cryptographic identity (keys stored as base64)
		public_key TEXT NOT NULL,
		private_key TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS attestations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		from_system_id TEXT NOT NULL,
		to_system_id TEXT NOT NULL,
		received_by TEXT NOT NULL DEFAULT '',
		timestamp INTEGER NOT NULL,
		message_type TEXT NOT NULL,
		signature TEXT NOT NULL,
		public_key TEXT NOT NULL,
		verified INTEGER DEFAULT 0,
		created_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS peer_systems (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	x REAL NOT NULL,
	y REAL NOT NULL,
	z REAL NOT NULL,
	star_class TEXT NOT NULL,
	star_color TEXT NOT NULL,
	star_description TEXT NOT NULL,
	peer_address TEXT NOT NULL DEFAULT '',
	sponsor_id TEXT,
	updated_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS peer_connections (
		system_id TEXT NOT NULL,
		peer_id TEXT NOT NULL,
		updated_at INTEGER NOT NULL,
		PRIMARY KEY (system_id, peer_id)
	);

	CREATE INDEX IF NOT EXISTS idx_peer_connections_updated ON peer_connections(updated_at);
	CREATE INDEX IF NOT EXISTS idx_system_coords ON system(x, y, z);
	CREATE INDEX IF NOT EXISTS idx_system_primary_class ON system(primary_class);
	CREATE INDEX IF NOT EXISTS idx_system_star_count ON system(star_count);
	CREATE INDEX IF NOT EXISTS idx_attestations_from ON attestations(from_system_id);
	CREATE INDEX IF NOT EXISTS idx_attestations_to ON attestations(to_system_id);
	CREATE INDEX IF NOT EXISTS idx_attestations_timestamp ON attestations(timestamp);
	CREATE INDEX IF NOT EXISTS idx_attestations_verified ON attestations(verified);

	-- Stellar Credits balance tracking
	CREATE TABLE IF NOT EXISTS credit_balance (
		system_id TEXT PRIMARY KEY,
		balance INTEGER NOT NULL DEFAULT 0,
		pending_credits REAL NOT NULL DEFAULT 0,
		total_earned INTEGER NOT NULL DEFAULT 0,
		total_sent INTEGER NOT NULL DEFAULT 0,
		total_received INTEGER NOT NULL DEFAULT 0,
		last_calculated INTEGER NOT NULL DEFAULT 0,
		longevity_start INTEGER NOT NULL DEFAULT 0,
		updated_at INTEGER NOT NULL
	);

	-- Credit transfer history (for future transfer feature)
	CREATE TABLE IF NOT EXISTS credit_transfers (
		id TEXT PRIMARY KEY,
		from_system_id TEXT NOT NULL,
		to_system_id TEXT NOT NULL,
		amount INTEGER NOT NULL,
		memo TEXT,
		timestamp INTEGER NOT NULL,
		signature TEXT NOT NULL,
		public_key TEXT NOT NULL,
		proof_hash TEXT,
		created_at INTEGER NOT NULL
	);

	-- Verified transfers from other systems (for double-spend prevention)
	CREATE TABLE IF NOT EXISTS verified_transfers (
		id TEXT PRIMARY KEY,
		from_system_id TEXT NOT NULL,
		to_system_id TEXT NOT NULL,
		amount INTEGER NOT NULL,
		timestamp INTEGER NOT NULL,
		signature TEXT NOT NULL,
		proof_hash TEXT NOT NULL,
		verified_at INTEGER NOT NULL
	);

	-- Identity bindings: locks UUID to public key on first contact
	-- Prevents UUID spoofing attacks
	CREATE TABLE IF NOT EXISTS identity_bindings (
		system_id TEXT PRIMARY KEY,
		public_key TEXT NOT NULL,
		first_seen INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_credit_transfers_from ON credit_transfers(from_system_id);
	CREATE INDEX IF NOT EXISTS idx_credit_transfers_to ON credit_transfers(to_system_id);
	CREATE INDEX IF NOT EXISTS idx_credit_transfers_timestamp ON credit_transfers(timestamp);
	CREATE INDEX IF NOT EXISTS idx_verified_transfers_from ON verified_transfers(from_system_id);
	CREATE INDEX IF NOT EXISTS idx_verified_transfers_to ON verified_transfers(to_system_id);
	`

	_, err := s.db.Exec(schema)
	if err != nil {
		return err
	}

	// Run migrations for existing databases
	return s.runMigrations()
}

// runMigrations handles schema updates for existing databases
func (s *Storage) runMigrations() error {
	// Add sponsor_id to system table if it doesn't exist
	s.db.Exec("ALTER TABLE system ADD COLUMN sponsor_id TEXT")
	
	// Add sponsor_id to peer_systems table if it doesn't exist
	s.db.Exec("ALTER TABLE peer_systems ADD COLUMN sponsor_id TEXT")
	
	// Add proof_hash to credit_transfers if it doesn't exist
	s.db.Exec("ALTER TABLE credit_transfers ADD COLUMN proof_hash TEXT")
	
	// Add received_by to attestations if it doesn't exist
	s.db.Exec("ALTER TABLE attestations ADD COLUMN received_by TEXT NOT NULL DEFAULT ''")
	s.db.Exec("CREATE INDEX IF NOT EXISTS idx_attestations_received_by ON attestations(received_by)")
	
	// Add pending_credits to credit_balance if it doesn't exist
	s.db.Exec("ALTER TABLE credit_balance ADD COLUMN pending_credits REAL NOT NULL DEFAULT 0")
	
	// Create verified_transfers table if it doesn't exist
	s.db.Exec(`CREATE TABLE IF NOT EXISTS verified_transfers (
		id TEXT PRIMARY KEY,
		from_system_id TEXT NOT NULL,
		to_system_id TEXT NOT NULL,
		amount INTEGER NOT NULL,
		timestamp INTEGER NOT NULL,
		signature TEXT NOT NULL,
		proof_hash TEXT NOT NULL,
		verified_at INTEGER NOT NULL
	)`)
	s.db.Exec("CREATE INDEX IF NOT EXISTS idx_verified_transfers_from ON verified_transfers(from_system_id)")
	s.db.Exec("CREATE INDEX IF NOT EXISTS idx_verified_transfers_to ON verified_transfers(to_system_id)")
	
	// Create identity_bindings table if it doesn't exist
	s.db.Exec(`CREATE TABLE IF NOT EXISTS identity_bindings (
		system_id TEXT PRIMARY KEY,
		public_key TEXT NOT NULL,
		first_seen INTEGER NOT NULL
	)`)
	
	return nil
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

	// Encode keys as base64
	publicKey := ""
	privateKey := ""
	if sys.Keys != nil {
		publicKey = base64.StdEncoding.EncodeToString(sys.Keys.PublicKey)
		privateKey = base64.StdEncoding.EncodeToString(sys.Keys.PrivateKey)
	}

	// Handle nullable sponsor_id
	var sponsorID *string
	if sys.SponsorID != nil {
		s := sys.SponsorID.String()
		sponsorID = &s
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO system (
			id, name, x, y, z,
			primary_class, primary_description, primary_color, primary_temperature, primary_luminosity,
			secondary_class, secondary_description, secondary_color, secondary_temperature, secondary_luminosity,
			tertiary_class, tertiary_description, tertiary_color, tertiary_temperature, tertiary_luminosity,
			is_binary, is_trinary, star_count,
			created_at, last_seen_at, address, peer_address, sponsor_id, public_key, private_key
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, sys.ID.String(), sys.Name, sys.X, sys.Y, sys.Z,
		sys.Stars.Primary.Class, sys.Stars.Primary.Description, sys.Stars.Primary.Color,
		sys.Stars.Primary.Temperature, sys.Stars.Primary.Luminosity,
		secondaryClass, secondaryDesc, secondaryColor, secondaryTemp, secondaryLum,
		tertiaryClass, tertiaryDesc, tertiaryColor, tertiaryTemp, tertiaryLum,
		isBinary, isTrinary, sys.Stars.Count,
		sys.CreatedAt.Unix(), sys.LastSeenAt.Unix(), sys.Address, sys.PeerAddress, sponsorID, publicKey, privateKey)
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
	var publicKeyB64, privateKeyB64, sponsorIDStr sql.NullString

	err := s.db.QueryRow(`
		SELECT id, name, x, y, z,
			primary_class, primary_description, primary_color, primary_temperature, primary_luminosity,
			secondary_class, secondary_description, secondary_color, secondary_temperature, secondary_luminosity,
			tertiary_class, tertiary_description, tertiary_color, tertiary_temperature, tertiary_luminosity,
			is_binary, is_trinary, star_count,
			created_at, last_seen_at, address, peer_address, sponsor_id, public_key, private_key
		FROM system LIMIT 1
	`).Scan(&idStr, &sys.Name, &sys.X, &sys.Y, &sys.Z,
		&sys.Stars.Primary.Class, &sys.Stars.Primary.Description, &sys.Stars.Primary.Color,
		&sys.Stars.Primary.Temperature, &sys.Stars.Primary.Luminosity,
		&secondaryClass, &secondaryDesc, &secondaryColor, &secondaryTemp, &secondaryLum,
		&tertiaryClass, &tertiaryDesc, &tertiaryColor, &tertiaryTemp, &tertiaryLum,
		&isBinary, &isTrinary, &starCount,
		&createdAt, &lastSeenAt, &sys.Address, &sys.PeerAddress, &sponsorIDStr, &publicKeyB64, &privateKeyB64)

	if err != nil {
		return nil, err
	}

	sys.ID = uuid.MustParse(idStr)
	sys.CreatedAt = time.Unix(createdAt, 0)
	sys.LastSeenAt = time.Unix(lastSeenAt, 0)

	// Load sponsor ID if present
	if sponsorIDStr.Valid && sponsorIDStr.String != "" {
		sponsorID := uuid.MustParse(sponsorIDStr.String)
		sys.SponsorID = &sponsorID
	}

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

	// Load cryptographic keys from database
	if publicKeyB64.Valid && privateKeyB64.Valid &&
	   publicKeyB64.String != "" && privateKeyB64.String != "" {
		// Decode stored keys
		publicKey, err := base64.StdEncoding.DecodeString(publicKeyB64.String)
		if err != nil {
			return nil, fmt.Errorf("failed to decode public key: %w", err)
		}
		privateKey, err := base64.StdEncoding.DecodeString(privateKeyB64.String)
		if err != nil {
			return nil, fmt.Errorf("failed to decode private key: %w", err)
		}

		sys.Keys = &KeyPair{
			PublicKey:  publicKey,
			PrivateKey: privateKey,
		}
	} else {
		// No keys stored (legacy database), generate new ones
		keys, err := GenerateKeyPair()
		if err != nil {
			return nil, fmt.Errorf("failed to generate keys: %w", err)
		}
		sys.Keys = keys
	}

	return &sys, nil
}

// Close closes the database connection
func (s *Storage) Close() error {
	return s.db.Close()
}

// SaveAttestation stores a cryptographically signed attestation
// receivedBy is the local system ID that received this attestation (for credit tracking)
func (s *Storage) SaveAttestation(attestation *Attestation, receivedBy uuid.UUID) error {
	verified := 0
	if attestation.Verify() {
		verified = 1
	}

	_, err := s.db.Exec(`
		INSERT INTO attestations (
			from_system_id, to_system_id, received_by, timestamp, message_type,
			signature, public_key, verified, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, attestation.FromSystemID.String(), attestation.ToSystemID.String(),
		receivedBy.String(), attestation.Timestamp, attestation.MessageType,
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

// SavePeerSystem caches a peer's full system info
func (s *Storage) SavePeerSystem(sys *System) error {
	// Handle nullable sponsor_id
	var sponsorID *string
	if sys.SponsorID != nil {
		s := sys.SponsorID.String()
		sponsorID = &s
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO peer_systems (
			id, name, x, y, z,
			star_class, star_color, star_description,
			peer_address, sponsor_id, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, sys.ID.String(), sys.Name, sys.X, sys.Y, sys.Z,
		sys.Stars.Primary.Class, sys.Stars.Primary.Color, sys.Stars.Primary.Description,
		sys.PeerAddress, sponsorID, time.Now().Unix())
	return err
}

// TouchPeerSystem updates only the timestamp for a peer system
// Used when we verify a system is alive without changing its info
func (s *Storage) TouchPeerSystem(systemID uuid.UUID) error {
	_, err := s.db.Exec(`UPDATE peer_systems SET updated_at = ? WHERE id = ?`,
		time.Now().Unix(), systemID.String())
	return err
}

// GetPeerSystem retrieves cached system info for a peer
func (s *Storage) GetPeerSystem(systemID uuid.UUID) (*System, error) {
	var sys System
	var idStr string
	var updatedAt int64
	var sponsorIDStr sql.NullString

	err := s.db.QueryRow(`
		SELECT id, name, x, y, z, star_class, star_color, star_description, peer_address, sponsor_id, updated_at
		FROM peer_systems WHERE id = ?
	`, systemID.String()).Scan(&idStr, &sys.Name, &sys.X, &sys.Y, &sys.Z,
		&sys.Stars.Primary.Class, &sys.Stars.Primary.Color, &sys.Stars.Primary.Description,
		&sys.PeerAddress, &sponsorIDStr, &updatedAt)

	if err != nil {
		return nil, err
	}

	sys.ID = uuid.MustParse(idStr)
	
	// Load sponsor ID if present
	if sponsorIDStr.Valid && sponsorIDStr.String != "" {
		sponsorID := uuid.MustParse(sponsorIDStr.String)
		sys.SponsorID = &sponsorID
	}
	
	return &sys, nil
}

// GetDatabaseStats returns current database statistics
func (s *Storage) GetDatabaseStats() (map[string]interface{}, error) {
    stats := make(map[string]interface{})

    // Count attestations
    var attestationCount int
    s.db.QueryRow("SELECT COUNT(*) FROM attestations").Scan(&attestationCount)
    stats["attestation_count"] = attestationCount

    // Count known systems
    var systemCount int
    s.db.QueryRow("SELECT COUNT(*) FROM peer_systems").Scan(&systemCount)
    stats["known_systems"] = systemCount

    // Database size
    var pageCount, pageSize int64
    s.db.QueryRow("SELECT page_count FROM pragma_page_count()").Scan(&pageCount)
    s.db.QueryRow("SELECT page_size FROM pragma_page_size()").Scan(&pageSize)
    stats["database_size_bytes"] = pageCount * pageSize

    // Oldest and newest attestation
    var oldest, newest int64
    s.db.QueryRow("SELECT MIN(timestamp) FROM attestations").Scan(&oldest)
    s.db.QueryRow("SELECT MAX(timestamp) FROM attestations").Scan(&newest)
    if oldest > 0 {
        stats["oldest_attestation"] = time.Unix(oldest, 0).Format(time.RFC3339)
    }
    if newest > 0 {
        stats["newest_attestation"] = time.Unix(newest, 0).Format(time.RFC3339)
    }

    return stats, nil
}

// CountKnownSystems returns the total number of unique systems we've heard about
func (s *Storage) CountKnownSystems() int {
    var count int
    err := s.db.QueryRow(`SELECT COUNT(*) FROM peer_systems`).Scan(&count)
    if err != nil {
        return 0
    }
    return count
}

// GetAllPeerSystems returns all cached peer system info (not just direct peers)
func (s *Storage) GetAllPeerSystems() ([]*System, error) {
    rows, err := s.db.Query(`
        SELECT id, name, x, y, z, star_class, star_color, star_description, peer_address, sponsor_id
        FROM peer_systems
    `)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var systems []*System
    for rows.Next() {
        var sys System
        var idStr string
        var peerAddress string
        var sponsorIDStr sql.NullString

        err := rows.Scan(&idStr, &sys.Name, &sys.X, &sys.Y, &sys.Z,
            &sys.Stars.Primary.Class, &sys.Stars.Primary.Color, &sys.Stars.Primary.Description,
            &peerAddress, &sponsorIDStr)
        if err != nil {
            continue
        }

        sys.ID = uuid.MustParse(idStr)
        sys.PeerAddress = peerAddress
        
        // Load sponsor ID if present
        if sponsorIDStr.Valid && sponsorIDStr.String != "" {
            sponsorID := uuid.MustParse(sponsorIDStr.String)
            sys.SponsorID = &sponsorID
        }
        
        systems = append(systems, &sys)
    }

    return systems, nil
}

// SavePeerConnections stores a system's peer list (learned from peer exchange)
func (s *Storage) SavePeerConnections(systemID uuid.UUID, peerIDs []uuid.UUID) error {
	now := time.Now().Unix()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO peer_connections (system_id, peer_id, updated_at)
		VALUES (?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, peerID := range peerIDs {
		_, err = stmt.Exec(systemID.String(), peerID.String(), now)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetAllConnections returns all known connections for map visualization
func (s *Storage) GetAllConnections(maxAge time.Duration) ([]TopologyEdge, error) {
	cutoff := time.Now().Add(-maxAge).Unix()

	rows, err := s.db.Query(`
		SELECT DISTINCT system_id, peer_id
		FROM peer_connections
		WHERE updated_at > ?
	`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	edgeMap := make(map[string]TopologyEdge)

	for rows.Next() {
		var fromID, toID string
		if err := rows.Scan(&fromID, &toID); err != nil {
			continue
		}

		// Deduplicate A→B and B→A
		var key string
		if fromID < toID {
			key = fromID + ":" + toID
		} else {
			key = toID + ":" + fromID
		}

		if _, exists := edgeMap[key]; !exists {
			edgeMap[key] = TopologyEdge{
				FromID:   fromID,
				FromName: s.getSystemName(fromID),
				ToID:     toID,
				ToName:   s.getSystemName(toID),
			}
		}
	}

	edges := make([]TopologyEdge, 0, len(edgeMap))
	for _, edge := range edgeMap {
		edges = append(edges, edge)
	}

	return edges, nil
}

// PrunePeerConnections removes stale connection data
func (s *Storage) PrunePeerConnections(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge).Unix()
	result, err := s.db.Exec(`DELETE FROM peer_connections WHERE updated_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// PrunePeerSystems removes stale peer system data
func (s *Storage) PrunePeerSystems(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge).Unix()
	result, err := s.db.Exec(`DELETE FROM peer_systems WHERE updated_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// TopologyEdge represents a connection between two systems
type TopologyEdge struct {
	FromID   string `json:"from_id"`
	FromName string `json:"from_name"`
	ToID     string `json:"to_id"`
	ToName   string `json:"to_name"`
}

// GetRecentTopology returns inferred connections from recent attestations
func (s *Storage) GetRecentTopology(maxAge time.Duration) ([]TopologyEdge, error) {
	cutoff := time.Now().Add(-maxAge).Unix()

	rows, err := s.db.Query(`
		SELECT DISTINCT from_system_id, to_system_id
		FROM attestations
		WHERE timestamp > ?
	`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Collect unique edges (deduplicate A→B and B→A)
	edgeMap := make(map[string]TopologyEdge)

	for rows.Next() {
		var fromID, toID string
		if err := rows.Scan(&fromID, &toID); err != nil {
			continue
		}

		// Create canonical edge key (smaller ID first) to deduplicate
		var key string
		if fromID < toID {
			key = fromID + ":" + toID
		} else {
			key = toID + ":" + fromID
		}

		if _, exists := edgeMap[key]; !exists {
			fromName := s.getSystemName(fromID)
			toName := s.getSystemName(toID)

			edgeMap[key] = TopologyEdge{
				FromID:   fromID,
				FromName: fromName,
				ToID:     toID,
				ToName:   toName,
			}
		}
	}

	// Convert map to slice
	edges := make([]TopologyEdge, 0, len(edgeMap))
	for _, edge := range edgeMap {
		edges = append(edges, edge)
	}

	return edges, nil
}

// getSystemName looks up a system name from peer_systems cache
func (s *Storage) getSystemName(systemID string) string {
	var name string
	err := s.db.QueryRow(`SELECT name FROM peer_systems WHERE id = ?`, systemID).Scan(&name)
	if err != nil {
		if len(systemID) >= 8 {
			return systemID[:8] + "..."
		}
		return systemID
	}
	return name
}

// =============================================================================
// STELLAR CREDITS STORAGE
// =============================================================================

// GetCreditBalance retrieves the credit balance for a system
func (s *Storage) GetCreditBalance(systemID uuid.UUID) (*CreditBalance, error) {
	var balance CreditBalance
	var updatedAt int64 // unused but needed for scan
	err := s.db.QueryRow(`
		SELECT system_id, balance, pending_credits, total_earned, total_sent, total_received, last_calculated, longevity_start, updated_at
		FROM credit_balance WHERE system_id = ?
	`, systemID.String()).Scan(
		&balance.SystemID,
		&balance.Balance,
		&balance.PendingCredits,
		&balance.TotalEarned,
		&balance.TotalSent,
		&balance.TotalReceived,
		&balance.LastUpdated,
		&balance.LongevityStart,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		// Return zero balance for new systems
		return &CreditBalance{
			SystemID:       systemID,
			Balance:        0,
			PendingCredits: 0,
			TotalEarned:    0,
			LastUpdated:    0,
			LongevityStart: 0,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return &balance, nil
}

// SaveCreditBalance persists a credit balance
func (s *Storage) SaveCreditBalance(balance *CreditBalance) error {
	_, err := s.db.Exec(`
		INSERT INTO credit_balance (system_id, balance, pending_credits, total_earned, total_sent, total_received, last_calculated, longevity_start, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(system_id) DO UPDATE SET
			balance = excluded.balance,
			pending_credits = excluded.pending_credits,
			total_earned = excluded.total_earned,
			total_sent = excluded.total_sent,
			total_received = excluded.total_received,
			last_calculated = excluded.last_calculated,
			longevity_start = excluded.longevity_start,
			updated_at = excluded.updated_at
	`, balance.SystemID.String(), balance.Balance, balance.PendingCredits, balance.TotalEarned,
		balance.TotalSent, balance.TotalReceived, balance.LastUpdated, balance.LongevityStart, time.Now().Unix())
	return err
}

// AddCredits adds earned credits to a system's balance
func (s *Storage) AddCredits(systemID uuid.UUID, amount int64) error {
	_, err := s.db.Exec(`
		INSERT INTO credit_balance (system_id, balance, total_earned, total_sent, total_received, last_calculated, longevity_start, updated_at)
		VALUES (?, ?, ?, 0, 0, ?, 0, ?)
		ON CONFLICT(system_id) DO UPDATE SET
			balance = balance + ?,
			total_earned = total_earned + ?,
			last_calculated = ?,
			updated_at = ?
	`, systemID.String(), amount, amount, time.Now().Unix(), time.Now().Unix(),
		amount, amount, time.Now().Unix(), time.Now().Unix())
	return err
}

// GetAttestationsSince retrieves attestations since a given timestamp
// Returns attestations where this system was the receiver (for credit calculation)
func (s *Storage) GetAttestationsSince(systemID uuid.UUID, since int64) ([]*Attestation, error) {
	rows, err := s.db.Query(`
		SELECT from_system_id, to_system_id, timestamp, message_type, signature, public_key
		FROM attestations
		WHERE received_by = ? AND timestamp > ?
		ORDER BY timestamp ASC
	`, systemID.String(), since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attestations []*Attestation
	for rows.Next() {
		var fromID, toID, msgType, sig, pubKey string
		var timestamp int64
		if err := rows.Scan(&fromID, &toID, &timestamp, &msgType, &sig, &pubKey); err != nil {
			continue
		}

		fromUUID, _ := uuid.Parse(fromID)
		toUUID, _ := uuid.Parse(toID)

		attestations = append(attestations, &Attestation{
			FromSystemID: fromUUID,
			ToSystemID:   toUUID,
			Timestamp:    timestamp,
			MessageType:  msgType,
			Signature:    sig,
			PublicKey:    pubKey,
		})
	}

	return attestations, nil
}

// SaveCreditTransfer persists a credit transfer
func (s *Storage) SaveCreditTransfer(transfer *CreditTransfer) error {
	var proofHash string
	if transfer.Proof != nil {
		proofHash = transfer.Proof.ProofHash()
	}
	
	_, err := s.db.Exec(`
		INSERT INTO credit_transfers (id, from_system_id, to_system_id, amount, memo, timestamp, signature, public_key, proof_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, transfer.ID.String(), transfer.FromSystemID.String(), transfer.ToSystemID.String(),
		transfer.Amount, transfer.Memo, transfer.Timestamp, transfer.Signature, transfer.PublicKey, proofHash, time.Now().Unix())
	return err
}

// GetCreditTransfers retrieves transfers for a system (sent or received)
func (s *Storage) GetCreditTransfers(systemID uuid.UUID, limit int) ([]*CreditTransfer, error) {
	rows, err := s.db.Query(`
		SELECT id, from_system_id, to_system_id, amount, memo, timestamp, signature, public_key
		FROM credit_transfers
		WHERE from_system_id = ? OR to_system_id = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, systemID.String(), systemID.String(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transfers []*CreditTransfer
	for rows.Next() {
		var t CreditTransfer
		var idStr, fromStr, toStr string
		if err := rows.Scan(&idStr, &fromStr, &toStr, &t.Amount, &t.Memo, &t.Timestamp, &t.Signature, &t.PublicKey); err != nil {
			continue
		}
		t.ID, _ = uuid.Parse(idStr)
		t.FromSystemID, _ = uuid.Parse(fromStr)
		t.ToSystemID, _ = uuid.Parse(toStr)
		transfers = append(transfers, &t)
	}

	return transfers, nil
}

// GetAllAttestationsForSystem retrieves all attestations where the system was the recipient
// Used for building credit proofs
func (s *Storage) GetAllAttestationsForSystem(systemID uuid.UUID) ([]*Attestation, error) {
	rows, err := s.db.Query(`
		SELECT from_system_id, to_system_id, timestamp, message_type, signature, public_key
		FROM attestations
		WHERE to_system_id = ?
		ORDER BY timestamp DESC
	`, systemID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attestations []*Attestation
	for rows.Next() {
		var fromID, toID, msgType, sig, pubKey string
		var timestamp int64
		if err := rows.Scan(&fromID, &toID, &timestamp, &msgType, &sig, &pubKey); err != nil {
			continue
		}

		fromUUID, _ := uuid.Parse(fromID)
		toUUID, _ := uuid.Parse(toID)

		attestations = append(attestations, &Attestation{
			FromSystemID: fromUUID,
			ToSystemID:   toUUID,
			Timestamp:    timestamp,
			MessageType:  msgType,
			Signature:    sig,
			PublicKey:    pubKey,
		})
	}

	return attestations, nil
}

// SaveVerifiedTransfer stores a transfer that we've verified from another system
func (s *Storage) SaveVerifiedTransfer(transfer *CreditTransfer) error {
	proofHash := ""
	if transfer.Proof != nil {
		proofHash = transfer.Proof.ProofHash()
	}
	
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO verified_transfers (id, from_system_id, to_system_id, amount, timestamp, signature, proof_hash, verified_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, transfer.ID.String(), transfer.FromSystemID.String(), transfer.ToSystemID.String(),
		transfer.Amount, transfer.Timestamp, transfer.Signature, proofHash, time.Now().Unix())
	return err
}

// GetVerifiedTransfersFrom retrieves all verified transfers sent by a system
// Used for double-spend detection
func (s *Storage) GetVerifiedTransfersFrom(systemID uuid.UUID) ([]*CreditTransfer, error) {
	rows, err := s.db.Query(`
		SELECT id, from_system_id, to_system_id, amount, timestamp, signature
		FROM verified_transfers
		WHERE from_system_id = ?
		ORDER BY timestamp DESC
	`, systemID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transfers []*CreditTransfer
	for rows.Next() {
		var t CreditTransfer
		var idStr, fromStr, toStr string
		if err := rows.Scan(&idStr, &fromStr, &toStr, &t.Amount, &t.Timestamp, &t.Signature); err != nil {
			continue
		}
		t.ID, _ = uuid.Parse(idStr)
		t.FromSystemID, _ = uuid.Parse(fromStr)
		t.ToSystemID, _ = uuid.Parse(toStr)
		transfers = append(transfers, &t)
	}

	return transfers, nil
}

// GetAllVerifiedTransfers retrieves all verified transfers we know about
// Used for comprehensive double-spend checking
func (s *Storage) GetAllVerifiedTransfers() ([]*CreditTransfer, error) {
	rows, err := s.db.Query(`
		SELECT id, from_system_id, to_system_id, amount, timestamp, signature
		FROM verified_transfers
		ORDER BY timestamp DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transfers []*CreditTransfer
	for rows.Next() {
		var t CreditTransfer
		var idStr, fromStr, toStr string
		if err := rows.Scan(&idStr, &fromStr, &toStr, &t.Amount, &t.Timestamp, &t.Signature); err != nil {
			continue
		}
		t.ID, _ = uuid.Parse(idStr)
		t.FromSystemID, _ = uuid.Parse(fromStr)
		t.ToSystemID, _ = uuid.Parse(toStr)
		transfers = append(transfers, &t)
	}

	return transfers, nil
}

// =============================================================================
// IDENTITY BINDING (UUID spoofing prevention)
// =============================================================================

// ValidateIdentityBinding checks if a system's public key matches what we've seen before
// Returns: (isValid bool, isNewIdentity bool, error)
// - If we've never seen this UUID: saves binding, returns (true, true, nil)
// - If we've seen it with same key: returns (true, false, nil)
// - If we've seen it with different key: returns (false, false, nil) - spoofing attempt
func (s *Storage) ValidateIdentityBinding(systemID uuid.UUID, publicKey string) (bool, bool, error) {
	var existingKey string
	err := s.db.QueryRow(
		"SELECT public_key FROM identity_bindings WHERE system_id = ?",
		systemID.String(),
	).Scan(&existingKey)

	if err == sql.ErrNoRows {
		// First time seeing this UUID - bind it
		_, err := s.db.Exec(
			"INSERT INTO identity_bindings (system_id, public_key, first_seen) VALUES (?, ?, ?)",
			systemID.String(), publicKey, time.Now().Unix(),
		)
		if err != nil {
			return false, false, fmt.Errorf("failed to save identity binding: %w", err)
		}
		return true, true, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("failed to check identity binding: %w", err)
	}

	// We've seen this UUID before - verify key matches
	if existingKey != publicKey {
		return false, false, nil // Spoofing attempt!
	}
	return true, false, nil
}