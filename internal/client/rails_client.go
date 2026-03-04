package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/juanmferreira93/iracing-agent/internal/config"
	"github.com/juanmferreira93/iracing-agent/internal/domain"
)

type RailsClient struct {
	baseURL    string
	apiKey     string
	uploadPath string
	healthPath string
	httpClient *http.Client
}

func NewRailsClient(cfg config.RailsConfig) *RailsClient {
	return &RailsClient{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:     cfg.APIKey,
		uploadPath: cfg.UploadPath,
		healthPath: cfg.HealthPath,
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
}

func (r *RailsClient) Ping(ctx context.Context) error {
	u, err := url.JoinPath(r.baseURL, r.healthPath)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", r.apiKey)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health endpoint returned %s", resp.Status)
	}

	return nil
}

func (r *RailsClient) UploadTelemetry(ctx context.Context, payload domain.UploadBundle) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	u, err := url.JoinPath(r.baseURL, r.uploadPath)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", r.apiKey)
	req.Header.Set("Idempotency-Key", payload.Session.ExternalID)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("upload failed: %s", resp.Status)
	}

	return nil
}
