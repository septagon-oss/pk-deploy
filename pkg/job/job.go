// Implements: REQ-INFRA-006.
// Per: ADR-0029.
// Discipline: C-14.

// Package job signs and verifies deployment jobs before workers execute them.
package job

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/septagon-oss/pk-deploy/pkg/deploy"
)

// Job is the unit a worker claims from a control plane.
type Job struct {
	ID        string                `json:"id"`
	Plan      deploy.Plan           `json:"plan"`
	Selector  deploy.WorkerSelector `json:"selector"`
	IssuedAt  time.Time             `json:"issuedAt"`
	ExpiresAt time.Time             `json:"expiresAt"`
	Nonce     string                `json:"nonce"`
}

// SignedJob carries a detached signature over the job plus signing metadata.
type SignedJob struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"keyId"`
	Job       Job    `json:"job"`
	Signature string `json:"signature"`
}

// KeyResolver returns the shared secret for a key ID.
type KeyResolver interface {
	ResolveKey(keyID string) ([]byte, error)
}

// KeyResolverFunc adapts a function to KeyResolver.
type KeyResolverFunc func(keyID string) ([]byte, error)

// ResolveKey implements KeyResolver.
func (fn KeyResolverFunc) ResolveKey(keyID string) ([]byte, error) {
	if fn == nil {
		return nil, errors.New("key resolver function is nil")
	}
	return fn(keyID)
}

// Validate rejects incomplete jobs before signing or execution.
func (j Job) Validate() error {
	var errs []error
	requireToken(&errs, "job.id", j.ID)
	requireToken(&errs, "job.nonce", j.Nonce)
	if j.IssuedAt.IsZero() {
		errs = append(errs, errors.New("job.issuedAt is required"))
	}
	if j.ExpiresAt.IsZero() {
		errs = append(errs, errors.New("job.expiresAt is required"))
	}
	if !j.IssuedAt.IsZero() && !j.ExpiresAt.IsZero() && !j.ExpiresAt.After(j.IssuedAt) {
		errs = append(errs, errors.New("job.expiresAt must be after issuedAt"))
	}
	if err := j.Plan.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("job.plan: %w", err))
	}
	return errors.Join(errs...)
}

func requireToken(errs *[]error, path, value string) {
	if err := validateToken(value); err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", path, err))
	}
}

func validateToken(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("is required")
	}
	if len(value) > 128 {
		return fmt.Errorf("length %d exceeds 128", len(value))
	}
	if !utf8.ValidString(value) {
		return errors.New("must be valid UTF-8")
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("contains invalid rune %q", r)
		}
	}
	return nil
}

// Expired reports whether the job is no longer executable.
func (j Job) Expired(now time.Time) bool {
	return !now.IsZero() && !j.ExpiresAt.IsZero() && !now.Before(j.ExpiresAt)
}

// Clone returns a deep copy of the job.
func (j Job) Clone() Job {
	j.Plan = j.Plan.Clone()
	j.Selector = j.Selector.Clone()
	return j
}

// Clone returns a deep copy of the signed job.
func (j SignedJob) Clone() SignedJob {
	j.Job = j.Job.Clone()
	return j
}
