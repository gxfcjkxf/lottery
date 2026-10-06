package recovery

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func validEvidence() Evidence {
	return Evidence{
		Replicas: 2, StreamingReplicas: 2, ReplicaReadOnlyRejects: 2, ReplicaDigestMatches: 2,
		InitialPrimaryWritable: true, PrimaryStoppedBeforePromotion: true,
		PromotedWritable: true, PromotedDigestMatches: true, PromotedExtraWriteVisible: true,
		RemainingReplicaReattached: true, RemainingReplicaDigestMatches: true,
		RestoredDigestMatches: true, RestoredImmutableGuardsVerified: true,
		BackupSHA256: strings.Repeat("a", 64), BackupBytes: 1,
		BaselineTableCount: 1, RestoredTableCount: 1, BaselineRowCount: 1, RestoredRowCount: 1,
		MigrationCount: 1, RestoreMigrationCount: 1, ExpectedMigrationCount: 1,
		OriginalDatabaseUntouched: true,
		ReplicaCatchupSeconds:     0, PromotionSeconds: 0, RestoreSeconds: 0,
	}
}

func TestValidateAcceptsCompleteManualDrillEvidence(t *testing.T) {
	if err := Validate(validEvidence()); err != nil {
		t.Fatalf("Validate(valid evidence) = %v", err)
	}
}

func TestEvidenceJSONTagsAreSnakeCase(t *testing.T) {
	typeOf := reflect.TypeOf(Evidence{})
	for i := 0; i < typeOf.NumField(); i++ {
		field := typeOf.Field(i)
		if got := field.Tag.Get("json"); got == "" || strings.Contains(got, "Upper") || got != strings.ToLower(got) {
			t.Errorf("%s has non-snake_case JSON tag %q", field.Name, got)
		}
	}
}

func TestValidateRejectsIncompleteOrOverstatedEvidence(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Evidence)
	}{
		{"replica count", func(e *Evidence) { e.Replicas = 1 }},
		{"streaming count", func(e *Evidence) { e.StreamingReplicas = 3 }},
		{"read-only rejects", func(e *Evidence) { e.ReplicaReadOnlyRejects = 1 }},
		{"replica digest checks", func(e *Evidence) { e.ReplicaDigestMatches = 1 }},
		{"initial primary writability", func(e *Evidence) { e.InitialPrimaryWritable = false }},
		{"old primary stop ordering", func(e *Evidence) { e.PrimaryStoppedBeforePromotion = false }},
		{"promoted writability", func(e *Evidence) { e.PromotedWritable = false }},
		{"promoted content digest", func(e *Evidence) { e.PromotedDigestMatches = false }},
		{"promoted extra write", func(e *Evidence) { e.PromotedExtraWriteVisible = false }},
		{"remaining replica reattachment", func(e *Evidence) { e.RemainingReplicaReattached = false }},
		{"remaining replica digest", func(e *Evidence) { e.RemainingReplicaDigestMatches = false }},
		{"restored content digest", func(e *Evidence) { e.RestoredDigestMatches = false }},
		{"restored immutable guards", func(e *Evidence) { e.RestoredImmutableGuardsVerified = false }},
		{"uppercase backup digest", func(e *Evidence) { e.BackupSHA256 = strings.Repeat("A", 64) }},
		{"short backup digest", func(e *Evidence) { e.BackupSHA256 = strings.Repeat("a", 63) }},
		{"invalid backup digest character", func(e *Evidence) { e.BackupSHA256 = strings.Repeat("a", 63) + "g" }},
		{"zero backup bytes", func(e *Evidence) { e.BackupBytes = 0 }},
		{"zero baseline tables", func(e *Evidence) { e.BaselineTableCount = 0 }},
		{"zero baseline rows", func(e *Evidence) { e.BaselineRowCount = 0 }},
		{"restored table count mismatch", func(e *Evidence) { e.RestoredTableCount++ }},
		{"restored row count mismatch", func(e *Evidence) { e.RestoredRowCount++ }},
		{"zero migration placeholders", func(e *Evidence) { e.MigrationCount, e.RestoreMigrationCount, e.ExpectedMigrationCount = 0, 0, 0 }},
		{"baseline migration mismatch", func(e *Evidence) { e.MigrationCount++ }},
		{"restored migration mismatch", func(e *Evidence) { e.RestoreMigrationCount++ }},
		{"expected migration mismatch", func(e *Evidence) { e.ExpectedMigrationCount++ }},
		{"original database modified", func(e *Evidence) { e.OriginalDatabaseUntouched = false }},
		{"automatic failover overclaim", func(e *Evidence) { e.AutomaticFailoverVerified = true }},
		{"production recovery overclaim", func(e *Evidence) { e.ProductionDisasterRecoveryAccepted = true }},
		{"negative catchup duration", func(e *Evidence) { e.ReplicaCatchupSeconds = -1 }},
		{"negative promotion duration", func(e *Evidence) { e.PromotionSeconds = -1 }},
		{"negative restore duration", func(e *Evidence) { e.RestoreSeconds = -1 }},
		{"NaN catchup duration", func(e *Evidence) { e.ReplicaCatchupSeconds = math.NaN() }},
		{"positive infinite promotion duration", func(e *Evidence) { e.PromotionSeconds = math.Inf(1) }},
		{"negative infinite restore duration", func(e *Evidence) { e.RestoreSeconds = math.Inf(-1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := validEvidence()
			tt.change(&e)
			if err := Validate(e); err == nil {
				t.Fatal("Validate accepted invalid evidence")
			}
		})
	}
}
