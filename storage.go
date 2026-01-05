package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type Storage struct {
	db *sql.DB
}

// AttestationSummary holds aggregated attestation data for a time period
type AttestationSummary struct {
    PeerSystemID      string
    Direction         string // "inbound" or "outbound"
    PeriodStart       int64
    PeriodEnd         int64
    HeartbeatCount    int
    PeerExchangeCount int
    RelayCount        int
    OtherCount        int
    SampleSignature   string
    SamplePublicKey   string
}

// TotalCount returns the total attestations in this summary
func (s *AttestationSummary) TotalCount() int {
    return s.HeartbeatCount + s.PeerExchangeCount + s.RelayCount + s.OtherCount
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
		peer_address TEXT NOT NULL,
		-- Cryptographic identity (keys stored as base64)
		public_key TEXT NOT NULL,
		private_key TEXT NOT NULL
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

	CREATE TABLE IF NOT EXISTS peer_systems (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	x REAL NOT NULL,
	y REAL NOT NULL,
	z REAL NOT NULL,
	star_class TEXT NOT NULL,
	star_color TEXT NOT NULL,
	star_description TEXT NOT NULL,
	updated_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS attestation_summaries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    peer_system_id TEXT NOT NULL,
    direction TEXT NOT NULL,
    period_start INTEGER NOT NULL,
    period_end INTEGER NOT NULL,
    heartbeat_count INTEGER DEFAULT 0,
    peer_exchange_count INTEGER DEFAULT 0,
    relay_count INTEGER DEFAULT 0,
    other_count INTEGER DEFAULT 0,
    sample_signature TEXT NOT NULL,
    sample_public_key TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE(peer_system_id, direction, period_start)
	);

	CREATE INDEX IF NOT EXISTS idx_summaries_peer ON attestation_summaries(peer_system_id);
	CREATE INDEX IF NOT EXISTS idx_summaries_period ON attestation_summaries(period_start);
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
	
	// Encode keys as base64
	publicKey := ""
	privateKey := ""
	if sys.Keys != nil {
		publicKey = base64.StdEncoding.EncodeToString(sys.Keys.PublicKey)
		privateKey = base64.StdEncoding.EncodeToString(sys.Keys.PrivateKey)
	}
	
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO system (
			id, name, x, y, z,
			primary_class, primary_description, primary_color, primary_temperature, primary_luminosity,
			secondary_class, secondary_description, secondary_color, secondary_temperature, secondary_luminosity,
			tertiary_class, tertiary_description, tertiary_color, tertiary_temperature, tertiary_luminosity,
			is_binary, is_trinary, star_count,
			created_at, last_seen_at, address, peer_address, public_key, private_key
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, sys.ID.String(), sys.Name, sys.X, sys.Y, sys.Z,
		sys.Stars.Primary.Class, sys.Stars.Primary.Description, sys.Stars.Primary.Color,
		sys.Stars.Primary.Temperature, sys.Stars.Primary.Luminosity,
		secondaryClass, secondaryDesc, secondaryColor, secondaryTemp, secondaryLum,
		tertiaryClass, tertiaryDesc, tertiaryColor, tertiaryTemp, tertiaryLum,
		isBinary, isTrinary, sys.Stars.Count,
		sys.CreatedAt.Unix(), sys.LastSeenAt.Unix(), sys.Address, sys.PeerAddress, publicKey, privateKey)
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
	var publicKeyB64, privateKeyB64 sql.NullString

	err := s.db.QueryRow(`
		SELECT id, name, x, y, z,
			primary_class, primary_description, primary_color, primary_temperature, primary_luminosity,
			secondary_class, secondary_description, secondary_color, secondary_temperature, secondary_luminosity,
			tertiary_class, tertiary_description, tertiary_color, tertiary_temperature, tertiary_luminosity,
			is_binary, is_trinary, star_count,
			created_at, last_seen_at, address, peer_address, public_key, private_key
		FROM system LIMIT 1
	`).Scan(&idStr, &sys.Name, &sys.X, &sys.Y, &sys.Z,
		&sys.Stars.Primary.Class, &sys.Stars.Primary.Description, &sys.Stars.Primary.Color,
		&sys.Stars.Primary.Temperature, &sys.Stars.Primary.Luminosity,
		&secondaryClass, &secondaryDesc, &secondaryColor, &secondaryTemp, &secondaryLum,
		&tertiaryClass, &tertiaryDesc, &tertiaryColor, &tertiaryTemp, &tertiaryLum,
		&isBinary, &isTrinary, &starCount,
		&createdAt, &lastSeenAt, &sys.Address, &sys.PeerAddress, &publicKeyB64, &privateKeyB64)

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

// SavePeerSystem caches a peer's full system info
func (s *Storage) SavePeerSystem(sys *System) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO peer_systems (
			id, name, x, y, z,
			star_class, star_color, star_description,
			updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, sys.ID.String(), sys.Name, sys.X, sys.Y, sys.Z,
		sys.Stars.Primary.Class, sys.Stars.Primary.Color, sys.Stars.Primary.Description,
		time.Now().Unix())
	return err
}

// GetPeerSystem retrieves cached system info for a peer
func (s *Storage) GetPeerSystem(systemID uuid.UUID) (*System, error) {
	var sys System
	var idStr string
	var updatedAt int64

	err := s.db.QueryRow(`
		SELECT id, name, x, y, z, star_class, star_color, star_description, updated_at
		FROM peer_systems WHERE id = ?
	`, systemID.String()).Scan(&idStr, &sys.Name, &sys.X, &sys.Y, &sys.Z,
		&sys.Stars.Primary.Class, &sys.Stars.Primary.Color, &sys.Stars.Primary.Description,
		&updatedAt)

	if err != nil {
		return nil, err
	}

	sys.ID = uuid.MustParse(idStr)
	return &sys, nil
}

// CompactionStats holds results of a compaction run
type CompactionStats struct {
    AttestationsProcessed int
    SummariesCreated      int
    AttestationsDeleted   int
    SpaceReclaimed        int64
}

// CompactAttestations aggregates old attestations into summaries
// keepDays specifies how many days of full attestations to retain
func (s *Storage) CompactAttestations(keepDays int) (*CompactionStats, error) {
    stats := &CompactionStats{}

    // Calculate cutoff timestamp
    cutoff := time.Now().AddDate(0, 0, -keepDays).Unix()

    // Get database size before compaction
    var sizeBefore int64
    err := s.db.QueryRow("SELECT page_count * page_size FROM pragma_page_count(), pragma_page_size()").Scan(&sizeBefore)
    if err != nil {
        sizeBefore = 0 // Non-critical, continue anyway
    }

    // Start transaction
    tx, err := s.db.Begin()
    if err != nil {
        return nil, fmt.Errorf("failed to start transaction: %w", err)
    }
    defer tx.Rollback()

    // Step 1: Find attestations to compact (older than cutoff)
    // Group by peer, direction (from/to us), and week
    rows, err := tx.Query(`
        SELECT
            CASE
                WHEN from_system_id = (SELECT id FROM system LIMIT 1) THEN to_system_id
                ELSE from_system_id
            END as peer_id,
            CASE
                WHEN from_system_id = (SELECT id FROM system LIMIT 1) THEN 'outbound'
                ELSE 'inbound'
            END as direction,
            (timestamp / 604800) * 604800 as week_start,
            message_type,
            COUNT(*) as count,
            MIN(timestamp) as period_start,
            MAX(timestamp) as period_end,
            MAX(signature) as sample_sig,
            MAX(public_key) as sample_key
        FROM attestations
        WHERE timestamp < ?
        GROUP BY peer_id, direction, week_start, message_type
    `, cutoff)
    if err != nil {
        return nil, fmt.Errorf("failed to query attestations: %w", err)
    }
    defer rows.Close()

    // Collect summaries to insert
    type summaryKey struct {
        peerID    string
        direction string
        weekStart int64
    }
    type summaryData struct {
        periodStart       int64
        periodEnd         int64
        heartbeatCount    int
        peerExchangeCount int
        relayCount        int
        otherCount        int
        sampleSig         string
        sampleKey         string
    }
    summaries := make(map[summaryKey]*summaryData)

    for rows.Next() {
        var peerID, direction, msgType, sampleSig, sampleKey string
        var weekStart, periodStart, periodEnd int64
        var count int

        err := rows.Scan(&peerID, &direction, &weekStart, &msgType, &count, &periodStart, &periodEnd, &sampleSig, &sampleKey)
        if err != nil {
            return nil, fmt.Errorf("failed to scan row: %w", err)
        }

        stats.AttestationsProcessed += count

        key := summaryKey{peerID, direction, weekStart}
        if summaries[key] == nil {
            summaries[key] = &summaryData{
                periodStart: periodStart,
                periodEnd:   periodEnd,
                sampleSig:   sampleSig,
                sampleKey:   sampleKey,
            }
        }

        // Update period bounds
        if periodStart < summaries[key].periodStart {
            summaries[key].periodStart = periodStart
        }
        if periodEnd > summaries[key].periodEnd {
            summaries[key].periodEnd = periodEnd
        }

        // Count by type
        switch msgType {
        case "heartbeat":
            summaries[key].heartbeatCount += count
        case "peer_exchange":
            summaries[key].peerExchangeCount += count
        case "relay":
            summaries[key].relayCount += count
        default:
            summaries[key].otherCount += count
        }
    }
    rows.Close()

    // Step 2: Insert summaries
    insertStmt, err := tx.Prepare(`
        INSERT OR REPLACE INTO attestation_summaries (
            peer_system_id, direction, period_start, period_end,
            heartbeat_count, peer_exchange_count, relay_count, other_count,
            sample_signature, sample_public_key, created_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `)
    if err != nil {
        return nil, fmt.Errorf("failed to prepare insert: %w", err)
    }
    defer insertStmt.Close()

    for key, data := range summaries {
        _, err := insertStmt.Exec(
            key.peerID, key.direction, key.weekStart, data.periodEnd,
            data.heartbeatCount, data.peerExchangeCount, data.relayCount, data.otherCount,
            data.sampleSig, data.sampleKey, time.Now().Unix(),
        )
        if err != nil {
            return nil, fmt.Errorf("failed to insert summary: %w", err)
        }
        stats.SummariesCreated++
    }

    // Step 3: Delete old attestations, but keep first and last per peer
    // First, identify the first and last attestation IDs per peer to preserve
    _, err = tx.Exec(`
        DELETE FROM attestations
        WHERE timestamp < ?
        AND rowid NOT IN (
            -- Keep first attestation per peer
            SELECT MIN(rowid) FROM attestations
            GROUP BY from_system_id, to_system_id
        )
        AND rowid NOT IN (
            -- Keep last attestation per peer
            SELECT MAX(rowid) FROM attestations
            GROUP BY from_system_id, to_system_id
        )
    `, cutoff)
    if err != nil {
        return nil, fmt.Errorf("failed to delete old attestations: %w", err)
    }

    // Get count of deleted rows
    var remainingCount int
    tx.QueryRow("SELECT changes()").Scan(&stats.AttestationsDeleted)
    tx.QueryRow("SELECT COUNT(*) FROM attestations").Scan(&remainingCount)

    // Commit transaction
    if err := tx.Commit(); err != nil {
        return nil, fmt.Errorf("failed to commit: %w", err)
    }

    // Step 4: VACUUM to reclaim space (must be outside transaction)
    _, err = s.db.Exec("VACUUM")
    if err != nil {
        // Non-fatal, just log it
        log.Printf("Warning: VACUUM failed: %v", err)
    }

    // Calculate space reclaimed
    var sizeAfter int64
    err = s.db.QueryRow("SELECT page_count * page_size FROM pragma_page_count(), pragma_page_size()").Scan(&sizeAfter)
    if err == nil && sizeBefore > 0 {
        stats.SpaceReclaimed = sizeBefore - sizeAfter
    }

    return stats, nil
}

// GetAttestationSummaries retrieves aggregated historical attestation data for a peer
func (s *Storage) GetAttestationSummaries(systemID uuid.UUID) ([]AttestationSummary, error) {
    rows, err := s.db.Query(`
        SELECT peer_system_id, direction, period_start, period_end,
               heartbeat_count, peer_exchange_count, relay_count, other_count,
               sample_signature, sample_public_key
        FROM attestation_summaries
        WHERE peer_system_id = ?
        ORDER BY period_start DESC
    `, systemID.String())
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var summaries []AttestationSummary
    for rows.Next() {
        var s AttestationSummary
        err := rows.Scan(
            &s.PeerSystemID, &s.Direction, &s.PeriodStart, &s.PeriodEnd,
            &s.HeartbeatCount, &s.PeerExchangeCount, &s.RelayCount, &s.OtherCount,
            &s.SampleSignature, &s.SamplePublicKey,
        )
        if err != nil {
            return nil, err
        }
        summaries = append(summaries, s)
    }

    return summaries, nil
}

// GetAllAttestationSummaries retrieves all historical summaries for reputation calculation
func (s *Storage) GetAllAttestationSummaries() ([]AttestationSummary, error) {
    rows, err := s.db.Query(`
        SELECT peer_system_id, direction, period_start, period_end,
               heartbeat_count, peer_exchange_count, relay_count, other_count,
               sample_signature, sample_public_key
        FROM attestation_summaries
        ORDER BY period_start DESC
    `)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var summaries []AttestationSummary
    for rows.Next() {
        var s AttestationSummary
        err := rows.Scan(
            &s.PeerSystemID, &s.Direction, &s.PeriodStart, &s.PeriodEnd,
            &s.HeartbeatCount, &s.PeerExchangeCount, &s.RelayCount, &s.OtherCount,
            &s.SampleSignature, &s.SamplePublicKey,
        )
        if err != nil {
            return nil, err
        }
        summaries = append(summaries, s)
    }

    return summaries, nil
}

// GetDatabaseStats returns current database statistics
func (s *Storage) GetDatabaseStats() (map[string]interface{}, error) {
    stats := make(map[string]interface{})

    // Count attestations
    var attestationCount int
    s.db.QueryRow("SELECT COUNT(*) FROM attestations").Scan(&attestationCount)
    stats["attestation_count"] = attestationCount

    // Count summaries
    var summaryCount int
    s.db.QueryRow("SELECT COUNT(*) FROM attestation_summaries").Scan(&summaryCount)
    stats["summary_count"] = summaryCount

    // Count peers
    var peerCount int
    s.db.QueryRow("SELECT COUNT(*) FROM peers").Scan(&peerCount)
    stats["peer_count"] = peerCount

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
        SELECT id, name, x, y, z, star_data, created_at, last_seen_at, address, peer_address
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
        var starData []byte
        var peerAddress sql.NullString

        err := rows.Scan(&idStr, &sys.Name, &sys.X, &sys.Y, &sys.Z, &starData,
            &sys.CreatedAt, &sys.LastSeenAt, &sys.Address, &peerAddress)
        if err != nil {
            continue
        }

        sys.ID = uuid.MustParse(idStr)
        if peerAddress.Valid {
            sys.PeerAddress = peerAddress.String
        }
        json.Unmarshal(starData, &sys.Stars)
        systems = append(systems, &sys)
    }

    return systems, nil
}