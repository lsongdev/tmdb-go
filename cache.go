package tmdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"
)

// Cache stores raw responses by request key (endpoint and query, without
// credentials). Implementations decide on expiry; a 404 is stored too, so
// missing items are not requested again.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool)
	Set(ctx context.Context, key string, value []byte)
}

// DirCache keeps one file per response in Dir. Entries older than TTL are
// ignored; a zero TTL keeps them forever.
type DirCache struct {
	Dir string
	TTL time.Duration
}

func (c DirCache) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	name := hex.EncodeToString(sum[:])
	return filepath.Join(c.Dir, name[:2], name+".json")
}

func (c DirCache) Get(_ context.Context, key string) ([]byte, bool) {
	name := c.path(key)
	info, err := os.Stat(name)
	if err != nil || c.TTL > 0 && time.Since(info.ModTime()) > c.TTL {
		return nil, false
	}
	data, err := os.ReadFile(name)
	return data, err == nil
}

func (c DirCache) Set(_ context.Context, key string, value []byte) {
	name := c.path(key)
	if os.MkdirAll(filepath.Dir(name), 0o755) != nil {
		return
	}
	tmp := name + ".tmp"
	if os.WriteFile(tmp, value, 0o644) == nil {
		_ = os.Rename(tmp, name)
	}
}
