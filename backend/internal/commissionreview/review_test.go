package commissionreview

import (
	"context"
	"errors"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestInspectRejectsInvalidAndUnpreparedInputs(t *testing.T) {
	ctx := context.Background()
	if _, err := Inspect(ctx, nil, "bad", "bad"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid input error=%v", err)
	}
	db := testdb.NewAtVersion(t, 73)
	brand := "0199a000-0000-7000-8000-000000000001"
	cycle := "0199a000-0000-7000-8000-000000009999"
	if _, err := Inspect(ctx, db, brand, cycle); !errors.Is(err, ErrCheckpoint) {
		t.Fatalf("unprepared history error=%v", err)
	}
	if err := database.PrepareCommissionHistoryReview(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(ctx, db, brand, cycle); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing cycle error=%v", err)
	}
	if _, err := Inspect(ctx, db, "0199A000-0000-7000-8000-000000000001", cycle); !errors.Is(err, ErrInvalid) {
		t.Fatalf("noncanonical identity accepted: %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := Inspect(canceled, db, brand, cycle); err == nil {
		t.Fatal("canceled context accepted")
	}
}
