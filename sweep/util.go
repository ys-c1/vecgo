package sweep

import (
	"crypto/sha256"
	"time"
)

func sha256Sum(b []byte) [32]byte { return sha256.Sum256(b) }

var now = func() time.Time { return time.Now() }

func since(start time.Time) int64 { return time.Since(start).Nanoseconds() }
