package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Index struct {
	mu           sync.RWMutex
	path         string
	fingerprints map[string]time.Time
}

func NewIndex(path string) (*Index, error) {
	idx := &Index{
		path:         path,
		fingerprints: map[string]time.Time{},
	}

	if err := idx.load(); err != nil {
		return nil, err
	}

	return idx, nil
}

func (i *Index) Seen(fingerprint string) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	_, ok := i.fingerprints[fingerprint]
	return ok
}

func (i *Index) Mark(fingerprint string) error {
	i.mu.Lock()
	i.fingerprints[fingerprint] = time.Now().UTC()
	i.mu.Unlock()
	return i.save()
}

func (i *Index) load() error {
	if err := os.MkdirAll(filepath.Dir(i.path), 0o755); err != nil {
		return err
	}

	data, err := os.ReadFile(i.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if len(data) == 0 {
		return nil
	}

	if err := json.Unmarshal(data, &i.fingerprints); err != nil {
		return fmt.Errorf("unmarshal state index: %w", err)
	}

	return nil
}

func (i *Index) save() error {
	i.mu.RLock()
	data, err := json.MarshalIndent(i.fingerprints, "", "  ")
	i.mu.RUnlock()
	if err != nil {
		return err
	}

	return os.WriteFile(i.path, data, 0o644)
}

func FileFingerprint(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	raw := fmt.Sprintf("%s|%d|%d", path, info.Size(), info.ModTime().UTC().UnixNano())
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:]), nil
}
