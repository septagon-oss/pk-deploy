// Implements: REQ-INFRA-006.
// Per: ADR-0029.
// Discipline: C-14.

package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/septagon-oss/pk-deploy/internal/controlplane"
	"github.com/septagon-oss/pk-deploy/internal/workerclient"
	"github.com/septagon-oss/pk-deploy/pkg/deploy"
	"github.com/septagon-oss/pk-deploy/pkg/evidence"
	"github.com/septagon-oss/pk-deploy/pkg/job"
	"github.com/septagon-oss/pk-deploy/pkg/metrics"
	"github.com/septagon-oss/pk-deploy/pkg/worker"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	registry := worker.NewRegistry()
	must(registry.Register("http.get", worker.ExecutorFunc(httpGetExecutor)))
	must(registry.Register("kubernetes.inventory", worker.ExecutorFunc(kubernetesInventoryExecutor)))
	must(registry.Register("kubernetes.set-image", worker.ExecutorFunc(kubernetesSetImageExecutor)))

	recorder := evidence.NewMemoryRecorder()
	var collector metrics.Collector
	runner := worker.Runner{
		Info:      cfg.info,
		Source:    workerclient.Source{BaseURL: cfg.controlURL},
		Verifier:  verifier(cfg.keyID, cfg.secret),
		Executors: registry,
		Evidence:  recorder,
		Metrics:   &collector,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}` + "\n"))
	})
	mux.Handle("GET /metrics", &collector)
	go func() {
		log.Printf("pk-deploy worker health server listening on %s", cfg.bindAddress)
		log.Fatal(http.ListenAndServe(cfg.bindAddress, mux))
	}()

	ticker := time.NewTicker(cfg.pollInterval)
	defer ticker.Stop()
	for {
		if err := runOnce(context.Background(), runner); err != nil {
			log.Printf("worker cycle: %v", err)
		}
		<-ticker.C
	}
}

type config struct {
	bindAddress  string
	controlURL   string
	keyID        string
	secret       []byte
	info         worker.Info
	pollInterval time.Duration
}

func loadConfig() (config, error) {
	secret, err := controlplaneSecret()
	if err != nil {
		return config{}, err
	}
	interval, err := time.ParseDuration(valueOr(os.Getenv("PK_DEPLOY_POLL_INTERVAL"), "5s"))
	if err != nil {
		return config{}, err
	}
	cfg := config{
		bindAddress:  valueOr(os.Getenv("PK_DEPLOY_WORKER_BIND"), ":8080"),
		controlURL:   strings.TrimSpace(os.Getenv("PK_DEPLOY_CONTROL_URL")),
		keyID:        valueOr(os.Getenv("PK_DEPLOY_KEY_ID"), "local"),
		secret:       secret,
		pollInterval: interval,
		info: worker.Info{
			ID:           valueOr(os.Getenv("PK_DEPLOY_WORKER_ID"), "pk-deploy-worker"),
			Capabilities: parseCSV(valueOr(os.Getenv("PK_DEPLOY_WORKER_CAPABILITIES"), "http.get,kubernetes.inventory,kubernetes.set-image")),
			Labels:       parseLabels(valueOr(os.Getenv("PK_DEPLOY_WORKER_LABELS"), "environment=staging")),
		},
	}
	if cfg.controlURL == "" {
		return config{}, errors.New("PK_DEPLOY_CONTROL_URL is required")
	}
	if cfg.pollInterval <= 0 {
		return config{}, errors.New("PK_DEPLOY_POLL_INTERVAL must be positive")
	}
	return cfg, nil
}

func controlplaneSecret() ([]byte, error) {
	return controlplane.SharedSecretFromEnv()
}

func runOnce(ctx context.Context, runner worker.Runner) error {
	result, err := runner.RunOnce(ctx)
	if err != nil {
		if errors.Is(err, worker.ErrNoJob) {
			return nil
		}
		return err
	}
	log.Printf("job=%s status=%s steps=%d", result.JobID, result.Status, len(result.Steps))
	return nil
}

func verifier(keyID string, secret []byte) worker.Verifier {
	return worker.VerifierFunc(func(_ context.Context, signed job.SignedJob) (job.Job, error) {
		if signed.KeyID != keyID {
			return job.Job{}, errors.New("unexpected key id")
		}
		return job.Verify(signed, job.KeyResolverFunc(func(string) ([]byte, error) {
			return secret, nil
		}), time.Now().UTC())
	})
}

func httpGetExecutor(ctx context.Context, req worker.ExecuteRequest) (deploy.StepResult, error) {
	url := strings.TrimSpace(req.Step.Inputs["url"])
	if url == "" {
		return deploy.StepResult{}, errors.New("http.get requires url input")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return deploy.StepResult{}, err
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return deploy.StepResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return deploy.StepResult{Message: resp.Status}, errors.New(resp.Status)
	}
	return deploy.StepResult{
		Message: resp.Status,
		Outputs: map[string]string{
			"status": resp.Status,
		},
	}, nil
}

func parseCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseLabels(raw string) map[string]string {
	out := map[string]string{}
	for _, item := range parseCSV(raw) {
		key, value, ok := strings.Cut(item, "=")
		if ok && strings.TrimSpace(key) != "" {
			out[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return out
}

func valueOr(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
