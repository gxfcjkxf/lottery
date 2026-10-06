// Package recovery defines evidence for the bounded manual recovery drill.
package recovery

import (
	"fmt"
	"math"
	"regexp"
)

var lowerSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Evidence records independently collected results from the isolated recovery
// drill. Digest match fields represent whole-table content digest checks; row
// counts alone do not establish that database contents match.
type Evidence struct {
	Replicas                           int64   `json:"replicas"`
	StreamingReplicas                  int64   `json:"streaming_replicas"`
	ReplicaReadOnlyRejects             int64   `json:"replica_read_only_rejects"`
	ReplicaDigestMatches               int64   `json:"replica_digest_matches"`
	InitialPrimaryWritable             bool    `json:"initial_primary_writable"`
	PrimaryStoppedBeforePromotion      bool    `json:"primary_stopped_before_promotion"`
	PromotedWritable                   bool    `json:"promoted_writable"`
	PromotedDigestMatches              bool    `json:"promoted_digest_matches"`
	PromotedExtraWriteVisible          bool    `json:"promoted_extra_write_visible"`
	RemainingReplicaReattached         bool    `json:"remaining_replica_reattached"`
	RemainingReplicaDigestMatches      bool    `json:"remaining_replica_digest_matches"`
	RestoredDigestMatches              bool    `json:"restored_digest_matches"`
	RestoredImmutableGuardsVerified    bool    `json:"restored_immutable_guards_verified"`
	BackupSHA256                       string  `json:"backup_sha256"`
	BackupBytes                        int64   `json:"backup_bytes"`
	BaselineTableCount                 int64   `json:"baseline_table_count"`
	RestoredTableCount                 int64   `json:"restored_table_count"`
	BaselineRowCount                   int64   `json:"baseline_row_count"`
	RestoredRowCount                   int64   `json:"restored_row_count"`
	MigrationCount                     int64   `json:"migration_count"`
	RestoreMigrationCount              int64   `json:"restore_migration_count"`
	ExpectedMigrationCount             int64   `json:"expected_migration_count"`
	OriginalDatabaseUntouched          bool    `json:"original_database_untouched"`
	AutomaticFailoverVerified          bool    `json:"automatic_failover_verified"`
	ProductionDisasterRecoveryAccepted bool    `json:"production_disaster_recovery_accepted"`
	ReplicaCatchupSeconds              float64 `json:"replica_catchup_seconds"`
	PromotionSeconds                   float64 `json:"promotion_seconds"`
	RestoreSeconds                     float64 `json:"restore_seconds"`
}

// Validate rejects incomplete or overstated evidence for the manual, bounded
// lab drill. It intentionally makes no claim about automatic failover or
// production disaster recovery.
func Validate(e Evidence) error {
	if e.Replicas != 2 || e.StreamingReplicas != 2 || e.ReplicaReadOnlyRejects != 2 || e.ReplicaDigestMatches != 2 {
		return fmt.Errorf("replica evidence must report exactly two replicas, streaming replicas, read-only rejects, and digest matches")
	}
	if !e.InitialPrimaryWritable {
		return fmt.Errorf("initial primary was not verified writable")
	}
	if !e.PrimaryStoppedBeforePromotion {
		return fmt.Errorf("old primary was not verified stopped before promotion")
	}
	if !e.PromotedWritable || !e.PromotedDigestMatches || !e.PromotedExtraWriteVisible {
		return fmt.Errorf("promoted primary write and digest evidence is incomplete")
	}
	if !e.RemainingReplicaReattached || !e.RemainingReplicaDigestMatches {
		return fmt.Errorf("remaining replica reattachment and digest evidence is incomplete")
	}
	if !e.RestoredDigestMatches {
		return fmt.Errorf("restored database digest was not verified")
	}
	if !e.RestoredImmutableGuardsVerified {
		return fmt.Errorf("restored immutable guards were not verified")
	}
	if !lowerSHA256Pattern.MatchString(e.BackupSHA256) {
		return fmt.Errorf("backup SHA-256 must be 64 lowercase hexadecimal characters")
	}
	if e.BackupBytes <= 0 {
		return fmt.Errorf("backup byte count must be positive")
	}
	if e.BaselineTableCount <= 0 || e.BaselineRowCount <= 0 {
		return fmt.Errorf("baseline table and row counts must be positive")
	}
	if e.RestoredTableCount != e.BaselineTableCount || e.RestoredRowCount != e.BaselineRowCount {
		return fmt.Errorf("restored table and row counts must match baseline")
	}
	if e.RestoreMigrationCount != e.MigrationCount {
		return fmt.Errorf("restored migration count must match baseline migration count")
	}
	if e.ExpectedMigrationCount <= 0 || e.MigrationCount != e.ExpectedMigrationCount || e.RestoreMigrationCount != e.ExpectedMigrationCount {
		return fmt.Errorf("baseline and restored migration counts must equal the positive expected count")
	}
	if !e.OriginalDatabaseUntouched {
		return fmt.Errorf("original database was not verified untouched")
	}
	if e.AutomaticFailoverVerified {
		return fmt.Errorf("automatic failover claims are outside this manual drill")
	}
	if e.ProductionDisasterRecoveryAccepted {
		return fmt.Errorf("production disaster recovery acceptance claims are outside this lab drill")
	}
	for name, seconds := range map[string]float64{
		"replica catchup": e.ReplicaCatchupSeconds,
		"promotion":       e.PromotionSeconds,
		"restore":         e.RestoreSeconds,
	} {
		if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
			return fmt.Errorf("%s duration must be finite and nonnegative", name)
		}
	}
	return nil
}
