package betting

import (
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalid     = errors.New("invalid betting input")
	ErrDenied      = errors.New("betting operation denied")
	ErrNotFound    = errors.New("betting resource not found")
	ErrVersion     = errors.New("betting configuration version changed")
	ErrClosed      = errors.New("betting period not accepting this operation")
	ErrLimit       = errors.New("betting limit exceeded")
	ErrState       = errors.New("bet order state conflict")
	ErrSnapshot    = errors.New("stored betting snapshot malformed")
	ErrBrandPaused = errors.New("brand is paused")
)

type Service struct{ DB *pgxpool.Pool }
