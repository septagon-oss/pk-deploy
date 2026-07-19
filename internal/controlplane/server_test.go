// Validates: REQ-INFRA-006.
// Per: ADR-0029.
// Discipline: C-14.

package controlplane

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/septagon-oss/pk-deploy/pkg/deploy"
	"github.com/septagon-oss/pk-deploy/pkg/job"
	"github.com/septagon-oss/pk-deploy/pkg/worker"
)

func TestServerInventoryJobClaimCompleteRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	server := newTestServer(t, now)

	createReq := httptest.NewRequest(http.MethodPost, "/api/jobs", encodeJSON(t, server.inventoryJob(now)))
	createReq.Header.Set("X-PK-Deploy-Admin-Token", "admin-token")
	createResp := httptest.NewRecorder()
	server.Handler().ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("inventory create status = %d, body = %s", createResp.Code, createResp.Body.String())
	}

	claimBody := encodeJSON(t, worker.Info{
		ID:           "worker-1",
		Capabilities: []string{kubernetesInventoryExecutor},
		Labels:       map[string]string{"environment": "staging"},
	})
	claimReq := httptest.NewRequest(http.MethodPost, "/api/claim", claimBody)
	claimResp := httptest.NewRecorder()
	server.Handler().ServeHTTP(claimResp, claimReq)
	if claimResp.Code != http.StatusOK {
		t.Fatalf("claim status = %d, body = %s", claimResp.Code, claimResp.Body.String())
	}
	var signed job.SignedJob
	if err := json.NewDecoder(claimResp.Body).Decode(&signed); err != nil {
		t.Fatalf("decode signed job: %v", err)
	}
	if signed.Job.ID == "" || signed.Signature == "" {
		t.Fatalf("claimed incomplete signed job: %#v", signed)
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/api/complete", encodeJSON(t, worker.Result{
		JobID:      signed.Job.ID,
		PlanID:     signed.Job.Plan.ID,
		Status:     deploy.StatusSucceeded,
		StartedAt:  now,
		FinishedAt: now.Add(time.Second),
		Steps: []deploy.StepResult{{
			StepID:     "kubernetes-inventory",
			Status:     deploy.StatusSucceeded,
			StartedAt:  now,
			FinishedAt: now.Add(time.Second),
		}},
	}))
	completeResp := httptest.NewRecorder()
	server.Handler().ServeHTTP(completeResp, completeReq)
	if completeResp.Code != http.StatusOK {
		t.Fatalf("complete status = %d, body = %s", completeResp.Code, completeResp.Body.String())
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	statusResp := httptest.NewRecorder()
	server.Handler().ServeHTTP(statusResp, statusReq)
	if statusResp.Code != http.StatusOK {
		t.Fatalf("status code = %d", statusResp.Code)
	}
	var snapshot Snapshot
	if err := json.NewDecoder(statusResp.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if len(snapshot.Pending) != 0 || len(snapshot.Completed) != 1 || len(snapshot.Evidence) != 1 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	if got := snapshot.Evidence[0].ApplicationID; got != inventoryApplicationID {
		t.Fatalf("evidence application id = %q", got)
	}
	if got := snapshot.Evidence[0].EnvironmentID; got != "staging" {
		t.Fatalf("evidence environment id = %q", got)
	}
	if len(snapshot.Evidence[0].Steps) != 1 || snapshot.Evidence[0].Steps[0].StepID != "kubernetes-inventory" {
		t.Fatalf("evidence steps were not preserved: %#v", snapshot.Evidence[0].Steps)
	}
}

func TestServerRejectsUnauthorizedMutation(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC))
	req := httptest.NewRequest(http.MethodPost, "/api/jobs", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusUnauthorized)
	}
}

func TestServerMetricsExposeControlPlaneCounters(t *testing.T) {
	t.Parallel()

	server := newTestServer(t, time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC))
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), "pk_deploy_control_pending_jobs") {
		t.Fatalf("metrics missing pending gauge:\n%s", resp.Body.String())
	}
}

func newTestServer(t *testing.T, now time.Time) *Server {
	t.Helper()
	server, err := NewServer(Config{
		BindAddress:         ":0",
		KeyID:               "local",
		Secret:              []byte(strings.Repeat("s", 32)),
		AdminToken:          "admin-token",
		EnvironmentID:       "staging",
		InventoryNamespaces: []string{"platformkit-staging"},
	}, NewStore(now))
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	server.now = func() time.Time { return now }
	return server
}

func encodeJSON(t *testing.T, value any) *bytes.Buffer {
	t.Helper()
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(value); err != nil {
		t.Fatalf("encode JSON: %v", err)
	}
	return &body
}
