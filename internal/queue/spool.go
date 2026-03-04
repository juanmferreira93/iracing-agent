package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/juanmferreira93/iracing-agent/internal/domain"
	"github.com/juanmferreira93/iracing-agent/internal/ports"
)

type Spool struct {
	dir        string
	maxRetries int
}

type Item struct {
	Attempt     int                 `json:"attempt"`
	NextAttempt time.Time           `json:"next_attempt"`
	Payload     domain.UploadBundle `json:"payload"`
	LastError   string              `json:"last_error,omitempty"`
}

func NewSpool(dir string, maxRetries int) (*Spool, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Spool{dir: dir, maxRetries: maxRetries}, nil
}

func (s *Spool) Enqueue(payload domain.UploadBundle, reason error) error {
	item := Item{
		Attempt:     0,
		NextAttempt: time.Now().UTC(),
		Payload:     payload,
		LastError:   errString(reason),
	}
	return s.write(item)
}

func (s *Spool) Drain(ctx context.Context, uploader ports.TelemetryUploader) error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}

	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		paths = append(paths, filepath.Join(s.dir, e.Name()))
	}
	sort.Strings(paths)

	now := time.Now().UTC()
	for _, p := range paths {
		item, err := s.read(p)
		if err != nil {
			_ = os.Remove(p)
			continue
		}
		if now.Before(item.NextAttempt) {
			continue
		}

		err = uploader.UploadTelemetry(ctx, item.Payload)
		if err == nil {
			_ = os.Remove(p)
			continue
		}

		item.Attempt++
		item.LastError = err.Error()
		if item.Attempt >= s.maxRetries {
			dead := filepath.Join(s.dir, "dead")
			_ = os.MkdirAll(dead, 0o755)
			_ = os.Rename(p, filepath.Join(dead, filepath.Base(p)))
			continue
		}

		backoff := time.Duration(1<<min(item.Attempt, 6)) * time.Minute
		item.NextAttempt = time.Now().UTC().Add(backoff)
		if err := s.writeAt(p, item); err != nil {
			continue
		}
	}

	return nil
}

func (s *Spool) write(item Item) error {
	prefix := item.Payload.Session.ExternalID
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	if prefix == "" {
		prefix = "noid"
	}
	name := fmt.Sprintf("%d-%s.json", time.Now().UnixNano(), prefix)
	path := filepath.Join(s.dir, name)
	return s.writeAt(path, item)
}

func (s *Spool) writeAt(path string, item Item) error {
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (s *Spool) read(path string) (Item, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Item{}, err
	}
	var item Item
	if err := json.Unmarshal(data, &item); err != nil {
		return Item{}, err
	}
	return item, nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
