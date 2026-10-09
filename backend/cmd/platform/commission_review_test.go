package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestParseCommissionReviewArgs(t *testing.T) {
	brandID := "01234567-89ab-cdef-0123-456789abcdef"
	cycleID := "fedcba98-7654-3210-fedc-ba9876543210"
	tests := []struct {
		name       string
		args       []string
		recognized bool
		wantErr    bool
		want       commissionReviewCommand
	}{
		{name: "prepare with explicit confirmation", args: []string{"prepare-commission-history-review", commissionReviewConfirmation}, recognized: true, want: commissionReviewCommand{prepare: true}},
		{name: "inspect canonical lowercase UUIDs", args: []string{"commission-history-review", brandID, cycleID}, recognized: true, want: commissionReviewCommand{brandID: brandID, cycleID: cycleID}},
		{name: "prepare requires confirmation", args: []string{"prepare-commission-history-review"}, recognized: true, wantErr: true},
		{name: "prepare rejects extra arguments", args: []string{"prepare-commission-history-review", commissionReviewConfirmation, "extra"}, recognized: true, wantErr: true},
		{name: "prepare rejects wrong confirmation", args: []string{"prepare-commission-history-review", "--confirm=other"}, recognized: true, wantErr: true},
		{name: "inspect rejects uppercase UUID", args: []string{"commission-history-review", "01234567-89AB-cdef-0123-456789abcdef", cycleID}, recognized: true, wantErr: true},
		{name: "inspect rejects malformed UUID", args: []string{"commission-history-review", "brand", cycleID}, recognized: true, wantErr: true},
		{name: "inspect requires both UUIDs", args: []string{"commission-history-review", brandID}, recognized: true, wantErr: true},
		{name: "inspect rejects extra arguments", args: []string{"commission-history-review", brandID, cycleID, "extra"}, recognized: true, wantErr: true},
		{name: "existing command remains unrecognized", args: []string{"migrate"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, recognized, err := parseCommissionReviewArgs(tt.args)
			if recognized != tt.recognized {
				t.Fatalf("recognized = %v, want %v", recognized, tt.recognized)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("command = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestWriteCommissionReviewJSON(t *testing.T) {
	var output bytes.Buffer
	want := map[string]any{"status": "prepared", "checkpoint": float64(74)}
	if err := writeCommissionReviewJSON(&output, struct {
		Status     string `json:"status"`
		Checkpoint int    `json:"checkpoint"`
	}{"prepared", 74}); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("decode output JSON: %v", err)
	}
	if len(got) != len(want) || got["status"] != want["status"] || got["checkpoint"] != want["checkpoint"] {
		t.Fatalf("decoded output = %#v, want %#v", got, want)
	}
}

type shortReviewWriter struct{}

func (shortReviewWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestWriteCommissionReviewJSONRejectsShortOutput(t *testing.T) {
	if err := writeCommissionReviewJSON(shortReviewWriter{}, map[string]bool{"review_only": true}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short output reported success: %v", err)
	}
}

func TestCommissionReviewInvalidArgsPrecedeConfiguration(t *testing.T) {
	previous := os.Args
	t.Cleanup(func() { os.Args = previous })
	t.Setenv("DATABASE_URL", "not-a-database-sensitive-test-only")
	for _, args := range [][]string{{"prepare-commission-history-review"}, {"commission-history-review", "not-uuid", "not-uuid"}} {
		os.Args = append([]string{"platform"}, args...)
		err := run(slog.New(slog.NewTextHandler(io.Discard, nil)))
		if err == nil || !strings.HasPrefix(err.Error(), "usage:") || strings.Contains(err.Error(), "sensitive-test-only") {
			t.Fatalf("invalid args opened configuration or leaked secrets: %v", err)
		}
	}
}

// Exercise the real executable dispatch in a bounded test subprocess. A
// regression must not hang CI or open the user's original development port.
func TestCommissionReviewRuntimeHelper(t *testing.T) {
	mode := os.Getenv("LOTTERY_TEST_REVIEW_RUNTIME")
	if mode == "" {
		return
	}
	os.Args = []string{"platform", mode}
	err := run(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		t.Fatal("partial checkpoint runtime unexpectedly succeeded")
	}
	fmt.Fprintln(os.Stdout, err.Error())
}

func TestCommissionReviewCheckpointRefusesAPIAndWorkerExecutable(t *testing.T) {
	db := testdb.NewAtVersion(t, 73)
	if err := database.PrepareCommissionHistoryReview(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := db.QueryRow(context.Background(), `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	dsn, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	q := dsn.Query()
	q.Set("search_path", schema)
	dsn.RawQuery = q.Encode()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"serve", "worker"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, self, "-test.run=^TestCommissionReviewRuntimeHelper$")
			command.Env = append(os.Environ(), "LOTTERY_TEST_REVIEW_RUNTIME="+mode, "APP_ENV=test", "DATABASE_URL="+dsn.String(), "DATABASE_READ_URL=", "DATABASE_READ_URLS=", "AUTH_KEY=", "AUTH_KEY_FILE=", "HTTP_ADDR=127.0.0.1:0")
			output, err := command.CombinedOutput()
			if err != nil || !strings.Contains(string(output), "database migration status unavailable or incompatible") {
				t.Fatalf("partial checkpoint executable was not stopped at schema admission: err=%v output=%s", err, output)
			}
		})
	}
}
