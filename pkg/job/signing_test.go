// Validates: REQ-INFRA-006.
// Per: ADR-0029.
// Discipline: C-14.

package job

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/septagon-oss/pk-deploy/pkg/deploy"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	secret := strings.Repeat("s", 32)
	signed, err := Sign(validJob(now), "local", []byte(secret))
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	verified, err := Verify(signed, KeyResolverFunc(func(keyID string) ([]byte, error) {
		if keyID != "local" {
			return nil, errors.New("unknown key")
		}
		return []byte(secret), nil
	}), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if verified.ID != "job-1" {
		t.Fatalf("verified job ID = %q", verified.ID)
	}
}

func TestVerifyRejectsTamperedJob(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	secret := []byte(strings.Repeat("s", 32))
	signed, err := Sign(validJob(now), "local", secret)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	signed.Job.Plan.Steps[0].Inputs["release"] = "admin"
	_, err = Verify(signed, staticKey("local", secret), now.Add(time.Minute))
	if err == nil || !strings.Contains(err.Error(), "invalid job signature") {
		t.Fatalf("Verify() error = %v, want signature error", err)
	}
}

func TestVerifyRejectsExpiredJob(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	secret := []byte(strings.Repeat("s", 32))
	signed, err := Sign(validJob(now), "local", secret)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	_, err = Verify(signed, staticKey("local", secret), now.Add(2*time.Hour))
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("Verify() error = %v, want expiry error", err)
	}
}

func TestSignRequiresStrongSecret(t *testing.T) {
	t.Parallel()

	_, err := Sign(validJob(time.Now()), "local", []byte("short"))
	if err == nil || !strings.Contains(err.Error(), "at least 32 bytes") {
		t.Fatalf("Sign() error = %v, want secret length error", err)
	}
}

func staticKey(want string, secret []byte) KeyResolver {
	return KeyResolverFunc(func(keyID string) ([]byte, error) {
		if keyID != want {
			return nil, errors.New("unknown key")
		}
		return secret, nil
	})
}

func validJob(now time.Time) Job {
	return Job{
		ID: "job-1",
		Plan: deploy.Plan{
			ID: "release-1",
			Application: deploy.Application{
				ID: "app",
			},
			Environment: deploy.Environment{
				ID: "staging",
			},
			Artifacts: []deploy.Artifact{{
				ID:     "image",
				Kind:   "oci-image",
				Ref:    "registry.example.com/app:v1",
				Digest: "sha256:8f14e45fceea167a5a36dedd4bea2543",
			}},
			Gates: []deploy.Gate{{
				ID:       "approval",
				Kind:     "manual",
				Required: true,
				Status:   deploy.StatusSucceeded,
			}},
			Steps: []deploy.Step{{
				ID:       "deploy",
				Executor: "flux",
				Action:   "reconcile",
				Inputs:   map[string]string{"release": "api"},
			}},
			CreatedAt: now,
		},
		Selector: deploy.WorkerSelector{
			Capabilities: []string{"flux"},
			Labels:       map[string]string{"cluster": "staging"},
		},
		IssuedAt:  now,
		ExpiresAt: now.Add(time.Hour),
		Nonce:     "nonce-1",
	}
}
