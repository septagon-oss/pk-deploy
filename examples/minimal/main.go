package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/septagon-oss/pk-deploy/pkg/deploy"
	"github.com/septagon-oss/pk-deploy/pkg/evidence"
	"github.com/septagon-oss/pk-deploy/pkg/job"
	"github.com/septagon-oss/pk-deploy/pkg/metrics"
	"github.com/septagon-oss/pk-deploy/pkg/worker"
)

func main() {
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	secret := []byte(strings.Repeat("s", 32))
	signed, err := job.Sign(sampleJob(now), "local", secret)
	must(err)

	source := &singleJobSource{job: signed}
	registry := worker.NewRegistry()
	must(registry.Register("print", worker.ExecutorFunc(func(_ context.Context, req worker.ExecuteRequest) (deploy.StepResult, error) {
		return deploy.StepResult{
			Message: "would deploy " + req.Plan.Application.ID,
			Outputs: map[string]string{
				"executor": req.Step.Executor,
				"action":   req.Step.Action,
			},
		}, nil
	})))

	recorder := evidence.NewMemoryRecorder()
	var collector metrics.Collector
	result, err := worker.Runner{
		Info:      worker.Info{ID: "local-worker", Capabilities: []string{"print"}},
		Source:    source,
		Verifier:  verifyWith(secret, now.Add(time.Minute)),
		Executors: registry,
		Evidence:  recorder,
		Metrics:   &collector,
		Clock:     func() time.Time { return now },
	}.RunOnce(context.Background())
	must(err)

	fmt.Printf("result=%s steps=%d evidence=%d\n", result.Status, len(result.Steps), len(recorder.Bundles()))
	must(collector.WritePrometheus(stdout{}))
}

type singleJobSource struct {
	job     job.SignedJob
	claimed bool
}

func (s *singleJobSource) Claim(context.Context, worker.Info) (job.SignedJob, error) {
	if s.claimed {
		return job.SignedJob{}, worker.ErrNoJob
	}
	s.claimed = true
	return s.job, nil
}

func (*singleJobSource) Complete(context.Context, worker.Result) error {
	return nil
}

type stdout struct{}

func (stdout) Write(p []byte) (int, error) {
	return fmt.Print(string(p))
}

func verifyWith(secret []byte, now time.Time) worker.Verifier {
	return worker.VerifierFunc(func(_ context.Context, signed job.SignedJob) (job.Job, error) {
		return job.Verify(signed, job.KeyResolverFunc(func(string) ([]byte, error) {
			return secret, nil
		}), now)
	})
}

func sampleJob(now time.Time) job.Job {
	return job.Job{
		ID: "job-local",
		Plan: deploy.Plan{
			ID: "release-local",
			Application: deploy.Application{
				ID: "example-app",
			},
			Environment: deploy.Environment{
				ID: "local",
			},
			Artifacts: []deploy.Artifact{{
				ID:     "image",
				Kind:   "oci-image",
				Ref:    "registry.example.com/example-app:v1",
				Digest: "sha256:8f14e45fceea167a5a36dedd4bea2543",
			}},
			Gates: []deploy.Gate{{
				ID:       "approved",
				Kind:     "manual",
				Required: true,
				Status:   deploy.StatusSucceeded,
			}},
			Steps: []deploy.Step{{
				ID:       "deploy",
				Executor: "print",
				Action:   "dry-run",
			}},
			CreatedAt: now,
		},
		Selector: deploy.WorkerSelector{
			Capabilities: []string{"print"},
		},
		IssuedAt:  now,
		ExpiresAt: now.Add(time.Hour),
		Nonce:     "nonce-local",
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
