package client

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juanmferreira93/iracing-agent/internal/domain"
)

type LogUploader struct {
	dumpDir string
}

func NewLogUploader(dumpDir string) *LogUploader {
	return &LogUploader{dumpDir: dumpDir}
}

func (l *LogUploader) UploadTelemetry(_ context.Context, payload domain.UploadBundle) error {
	preview := struct {
		Session      domain.Session `json:"session"`
		LapsCount    int            `json:"laps_count"`
		SamplesCount int            `json:"samples_count"`
		LapsPreview  []domain.Lap   `json:"laps_preview"`
		FirstSample  *domain.Sample `json:"first_sample,omitempty"`
		LastSample   *domain.Sample `json:"last_sample,omitempty"`
	}{
		Session:      payload.Session,
		LapsCount:    len(payload.Laps),
		SamplesCount: len(payload.Samples),
		LapsPreview:  takeLaps(payload.Laps, 5),
	}

	if len(payload.Samples) > 0 {
		first := payload.Samples[0]
		last := payload.Samples[len(payload.Samples)-1]
		preview.FirstSample = &first
		preview.LastSample = &last
	}

	encoded, err := json.Marshal(preview)
	if err != nil {
		return err
	}

	log.Printf("log-only mode: parsed telemetry preview=%s", string(encoded))

	if err := l.writeFullPayload(payload); err != nil {
		return err
	}
	return nil
}

func (l *LogUploader) writeFullPayload(payload domain.UploadBundle) error {
	if l.dumpDir == "" {
		return nil
	}

	if err := os.MkdirAll(l.dumpDir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	prefix := payload.Session.ExternalID
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	if prefix == "" {
		prefix = "noid"
	}

	filename := time.Now().UTC().Format("20060102-150405") + "-" + sanitize(prefix) + ".json"
	fullPath := filepath.Join(l.dumpDir, filename)
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		return err
	}

	log.Printf("log-only mode: full parsed payload saved to %s", fullPath)
	return nil
}

func sanitize(input string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "-", "*", "", "?", "", "\"", "", "<", "", ">", "", "|", "")
	return replacer.Replace(input)
}

func takeLaps(laps []domain.Lap, max int) []domain.Lap {
	if len(laps) <= max {
		return laps
	}
	out := make([]domain.Lap, max)
	copy(out, laps[:max])
	return out
}
