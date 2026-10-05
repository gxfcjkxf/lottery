// Package mutation executes transactional, encrypted, persistent idempotent mutations.
package mutation

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"regexp"
)

type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Result struct {
	Status int             `json:"status"`
	Data   json.RawMessage `json:"data,omitempty"`
	Error  *Failure        `json:"error,omitempty"`
}

func OK(status int, data any) Result {
	b, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	return Result{Status: status, Data: b}
}
func Fail(status int, code, message string) Result {
	return Result{Status: status, Error: &Failure{code, message}}
}

type Engine struct {
	DB    *pgxpool.Pool
	key   []byte
	aead  cipher.AEAD
	slots chan struct{}
}

func New(db *pgxpool.Pool, key []byte) (*Engine, error) {
	if len(key) != 32 {
		return nil, errors.New("AUTH_KEY must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	capacity := 4
	if db != nil {
		if db.Config().MaxConns < 4 {
			return nil, errors.New("authenticated mutations require DB_MAX_CONNS >= 4")
		}
		capacity = int(db.Config().MaxConns / 2)
	}
	return &Engine{DB: db, key: append([]byte(nil), key...), aead: aead, slots: make(chan struct{}, capacity)}, nil
}
func (e *Engine) Fingerprint(value string) string {
	h := hmac.New(sha256.New, e.key)
	h.Write([]byte(value))
	return hex.EncodeToString(h.Sum(nil))
}
func (e *Engine) seal(body []byte) ([]byte, error) {
	return e.sealBound(body, []byte("lottery-idempotency-v1"))
}
func (e *Engine) sealBound(body, aad []byte) ([]byte, error) {
	nonce := make([]byte, e.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return e.aead.Seal(nonce, nonce, body, aad), nil
}
func (e *Engine) open(body []byte) ([]byte, error) {
	return e.openBound(body, []byte("lottery-idempotency-v1"))
}
func (e *Engine) openBound(body, aad []byte) ([]byte, error) {
	n := e.aead.NonceSize()
	if len(body) < n {
		return nil, errors.New("invalid sealed response")
	}
	return e.aead.Open(nil, body[:n], body[n:], aad)
}

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_:.-]{8,128}$`)

func validKey(key string) bool { return keyPattern.MatchString(key) }
func actorUUID(actor string) string {
	h := sha256.Sum256([]byte(actor))
	return fmt.Sprintf("%x-%x-%x-%x-%x", h[:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

type sealedResponse struct {
	Ciphertext string `json:"ciphertext"`
}

// Execute locks the logical operation, rolls back business failures to a savepoint,
// and commits the final result together with the business state. Transient DB errors
// aren't cached. Sensitive response bodies are AEAD-encrypted, never plaintext.
func (e *Engine) Execute(ctx context.Context, brand, actor, operation, key, fingerprint string, run func(context.Context, pgx.Tx) (Result, error)) (Result, error) {
	return e.execute(ctx, brand, actor, operation, key, fingerprint, nil, run)
}

// ExecuteChecked validates a caller's current session under transactional locks
// before looking up a cached response. Revoked credentials cannot replay data.
// check must be repeatable: it runs before and after waiting for the operation
// lock, so a naturally expired session cannot replay after a long lock wait.
func (e *Engine) ExecuteChecked(ctx context.Context, brand, actor, operation, key, fingerprint string, check func(context.Context, pgx.Tx) error, run func(context.Context, pgx.Tx) (Result, error)) (Result, error) {
	if check == nil {
		return Result{}, errors.New("checked mutation requires authorization check")
	}
	return e.execute(ctx, brand, actor, operation, key, fingerprint, check, run)
}
func (e *Engine) execute(ctx context.Context, brand, actor, operation, key, fingerprint string, check func(context.Context, pgx.Tx) error, run func(context.Context, pgx.Tx) (Result, error)) (Result, error) {
	if !validKey(key) {
		return Fail(400, "IDEMPOTENCY_KEY_INVALID", "需要 8 至 128 字符的 Idempotency-Key"), nil
	}
	// Reserve pool capacity for independent rate/challenge transactions. Without
	// this bound, an auth burst can occupy every connection and deadlock itself.
	select {
	case e.slots <- struct{}{}:
		defer func() { <-e.slots }()
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
	tx, err := e.DB.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx)
	if check != nil {
		if err = check(ctx, tx); err != nil {
			return Result{}, err
		}
	}
	actorID := actorUUID(actor)
	aad, _ := json.Marshal([]string{"lottery-idempotency-v1", brand, actorID, operation, key, fingerprint})
	lock := sha256.Sum256([]byte(brand + ":" + actorID + ":" + operation + ":" + key))
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(binary.BigEndian.Uint64(lock[:8]))); err != nil {
		return Result{}, err
	}
	if check != nil {
		if err = check(ctx, tx); err != nil {
			return Result{}, err
		}
	}
	var storedHash string
	var sealed []byte
	err = tx.QueryRow(ctx, `SELECT request_hash,response FROM idempotency_requests WHERE brand_id=$1 AND actor_id=$2 AND operation=$3 AND key=$4`, brand, actorID, operation, key).Scan(&storedHash, &sealed)
	if err == nil {
		if storedHash != fingerprint {
			return Fail(409, "IDEMPOTENCY_CONFLICT", "幂等键已用于不同请求"), nil
		}
		var stored sealedResponse
		if err = json.Unmarshal(sealed, &stored); err != nil {
			return Result{}, err
		}
		blob, err := base64.RawStdEncoding.DecodeString(stored.Ciphertext)
		if err != nil {
			return Result{}, err
		}
		plain, err := e.openBound(blob, aad)
		if err != nil {
			return Result{}, err
		}
		var result Result
		err = json.Unmarshal(plain, &result)
		return result, err
	}
	if err != pgx.ErrNoRows {
		return Result{}, err
	}
	if _, err = tx.Exec(ctx, "SAVEPOINT business"); err != nil {
		return Result{}, err
	}
	result, err := run(ctx, tx)
	if err != nil {
		return Result{}, err
	}
	if result.Status >= 400 {
		if _, err = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT business"); err != nil {
			return Result{}, err
		}
	}
	plain, err := json.Marshal(result)
	if err != nil {
		return Result{}, err
	}
	blob, err := e.sealBound(plain, aad)
	if err != nil {
		return Result{}, err
	}
	encoded, _ := json.Marshal(sealedResponse{base64.RawStdEncoding.EncodeToString(blob)})
	if _, err = tx.Exec(ctx, `INSERT INTO idempotency_requests(brand_id,actor_id,operation,key,request_hash,status_code,response) VALUES($1,$2,$3,$4,$5,$6,$7)`, brand, actorID, operation, key, fingerprint, result.Status, encoded); err != nil {
		return Result{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return result, nil
}
