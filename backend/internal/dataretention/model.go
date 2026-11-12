package dataretention

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalid  = errors.New("invalid data retention request")
	ErrVersion  = errors.New("data retention version conflict")
	ErrNotFound = errors.New("data retention task not found")
)

type Service struct{ DB *pgxpool.Pool }
type Policy struct {
	Schedule
	Version   int64     `json:"version"`
	Enabled   bool      `json:"enabled"`
	Timezone  string    `json:"timezone"`
	NextRun   *Run      `json:"next_run"`
	UpdatedAt time.Time `json:"updated_at"`
}
type PolicyInput struct {
	Schedule
	Version int64 `json:"version"`
	Enabled bool  `json:"enabled"`
}
type Task struct {
	BrandName     string            `json:"brand_name"`
	ID            string            `json:"id"`
	BrandID       string            `json:"brand_id"`
	PolicyVersion int64             `json:"policy_version"`
	Timezone      string            `json:"timezone"`
	ExecuteAt     time.Time         `json:"execute_at"`
	CutoffAt      time.Time         `json:"cutoff_at"`
	State         string            `json:"state"`
	Counts        map[string]Counts `json:"counts"`
	ErrorCode     string            `json:"error_code"`
	CreatedAt     time.Time         `json:"created_at"`
	FinishedAt    *time.Time        `json:"finished_at"`
}
type Counts struct {
	Deleted  string `json:"deleted"`
	Receipts string `json:"receipts"`
	Deferred string `json:"deferred"`
}
type Page struct {
	Items      []Task `json:"items"`
	TotalCount string `json:"total_count"`
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
}
