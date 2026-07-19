// Validates: REQ-INFRA-006.
// Per: ADR-0029.
// Discipline: C-14.

package workerclient

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/septagon-oss/pk-deploy/pkg/deploy"
	"github.com/septagon-oss/pk-deploy/pkg/job"
	"github.com/septagon-oss/pk-deploy/pkg/worker"
)

func TestSourceClaimMapsNotFoundToNoJob(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no job", http.StatusNotFound)
	}))
	defer server.Close()

	_, err := Source{BaseURL: server.URL}.Claim(t.Context(), worker.Info{ID: "worker"})
	if !errors.Is(err, worker.ErrNoJob) {
		t.Fatalf("Claim() error = %v, want ErrNoJob", err)
	}
}

func TestSourceClaimAndComplete(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	signed := job.SignedJob{
		Algorithm: job.AlgorithmHMACSHA256,
		KeyID:     "local",
		Job: job.Job{
			ID: "job-1",
			Plan: deploy.Plan{
				ID:          "plan-1",
				Application: deploy.Application{ID: "app"},
				Environment: deploy.Environment{ID: "staging"},
				Steps:       []deploy.Step{{ID: "inventory", Executor: "kubernetes.inventory", Action: "list-deployments"}},
				CreatedAt:   now,
			},
			IssuedAt:  now,
			ExpiresAt: now.Add(time.Minute),
			Nonce:     "nonce",
		},
		Signature: "signature",
	}
	completed := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/claim":
			_ = json.NewEncoder(w).Encode(signed)
		case "/api/complete":
			completed = true
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	source := Source{BaseURL: server.URL}
	claimed, err := source.Claim(t.Context(), worker.Info{ID: "worker"})
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if claimed.Job.ID != "job-1" {
		t.Fatalf("claimed job = %q", claimed.Job.ID)
	}
	if err := source.Complete(t.Context(), worker.Result{JobID: "job-1", Status: deploy.StatusSucceeded}); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if !completed {
		t.Fatal("complete endpoint was not called")
	}
}
