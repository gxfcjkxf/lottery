package ids

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

// New returns a time-ordered UUIDv7 with cryptographically random lower bits.
func New() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("UUID entropy unavailable")
	}
	now := uint64(time.Now().UnixMilli())
	var timestamp [8]byte
	binary.BigEndian.PutUint64(timestamp[:], now)
	copy(b[:6], timestamp[2:])
	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
