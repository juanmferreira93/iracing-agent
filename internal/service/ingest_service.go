package service

import (
	"context"
	"log"
	"time"

	"github.com/juanmferreira93/iracing-agent/internal/config"
	"github.com/juanmferreira93/iracing-agent/internal/ports"
	"github.com/juanmferreira93/iracing-agent/internal/queue"
	"github.com/juanmferreira93/iracing-agent/internal/state"
	"github.com/juanmferreira93/iracing-agent/internal/watcher"
)

type IngestService struct {
	cfg      config.Config
	watcher  *watcher.FileWatcher
	parser   ports.TelemetryParser
	uploader ports.TelemetryUploader
	index    *state.Index
	spool    *queue.Spool
}

func New(
	cfg config.Config,
	w *watcher.FileWatcher,
	p ports.TelemetryParser,
	u ports.TelemetryUploader,
	idx *state.Index,
	spool *queue.Spool,
) *IngestService {
	return &IngestService{
		cfg:      cfg,
		watcher:  w,
		parser:   p,
		uploader: u,
		index:    idx,
		spool:    spool,
	}
}

func (s *IngestService) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.Agent.ScanInterval())
	defer ticker.Stop()

	if err := s.cycle(ctx); err != nil {
		log.Printf("initial cycle error: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.cycle(ctx); err != nil {
				log.Printf("cycle error: %v", err)
			}
		}
	}
}

func (s *IngestService) ImportOne(ctx context.Context, filePath string) error {
	bundle, err := s.parser.ParseFile(ctx, filePath)
	if err != nil {
		return err
	}

	if err := s.uploader.UploadTelemetry(ctx, *bundle); err != nil {
		return err
	}

	fingerprint, err := state.FileFingerprint(filePath)
	if err != nil {
		return err
	}
	return s.index.Mark(fingerprint)
}

func (s *IngestService) cycle(ctx context.Context) error {
	if err := s.spool.Drain(ctx, s.uploader); err != nil {
		log.Printf("spool drain error: %v", err)
	}

	files, err := s.watcher.Discover(s.index)
	if err != nil {
		return err
	}

	for _, filePath := range files {
		if err := s.handleFile(ctx, filePath); err != nil {
			log.Printf("file ingest failed (%s): %v", filePath, err)
		}
	}

	return nil
}

func (s *IngestService) handleFile(ctx context.Context, filePath string) error {
	bundle, err := s.parser.ParseFile(ctx, filePath)
	if err != nil {
		fingerprint, fpErr := state.FileFingerprint(filePath)
		if fpErr == nil {
			_ = s.index.Mark(fingerprint)
		}
		return err
	}

	err = s.uploader.UploadTelemetry(ctx, *bundle)
	fingerprint, fpErr := state.FileFingerprint(filePath)
	if fpErr != nil {
		return fpErr
	}

	if err != nil {
		if qErr := s.spool.Enqueue(*bundle, err); qErr != nil {
			return qErr
		}
		return s.index.Mark(fingerprint)
	}

	return s.index.Mark(fingerprint)
}
