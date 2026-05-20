package workerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/septagon-oss/pk-deploy/pkg/job"
	"github.com/septagon-oss/pk-deploy/pkg/worker"
)

// Source claims and completes jobs through the control-plane HTTP API.
type Source struct {
	BaseURL string
	Client  *http.Client
}

// Claim implements worker.Source.
func (s Source) Claim(ctx context.Context, info worker.Info) (job.SignedJob, error) {
	var signed job.SignedJob
	status, err := s.postJSON(ctx, "/api/claim", info, &signed)
	if err != nil {
		return job.SignedJob{}, err
	}
	if status == http.StatusNotFound {
		return job.SignedJob{}, worker.ErrNoJob
	}
	if status < 200 || status >= 300 {
		return job.SignedJob{}, fmt.Errorf("claim job returned HTTP %d", status)
	}
	return signed, nil
}

// Complete implements worker.Source.
func (s Source) Complete(ctx context.Context, result worker.Result) error {
	status, err := s.postJSON(ctx, "/api/complete", result, nil)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("complete job returned HTTP %d", status)
	}
	return nil
}

func (s Source) postJSON(ctx context.Context, path string, request any, response any) (int, error) {
	if strings.TrimSpace(s.BaseURL) == "" {
		return 0, errors.New("control-plane base URL is required")
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(request); err != nil {
		return 0, fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.BaseURL, "/")+path, &body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if response != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
			return resp.StatusCode, fmt.Errorf("decode response: %w", err)
		}
	}
	return resp.StatusCode, nil
}
