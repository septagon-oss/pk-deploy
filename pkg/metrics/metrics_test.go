// Validates: REQ-INFRA-006.
// Per: ADR-0029.
// Discipline: C-14.

package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/septagon-oss/pk-deploy/pkg/deploy"
)

func TestCollectorRecordsLifecycle(t *testing.T) {
	t.Parallel()

	var collector Collector
	collector.JobStarted()
	collector.JobFinished(deploy.StatusSucceeded, 1500*time.Millisecond)

	snapshot := collector.Snapshot()
	if snapshot.Started != 1 || snapshot.Succeeded != 1 || snapshot.Failed != 0 || snapshot.Active != 0 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	if snapshot.DurationCount != 1 || snapshot.DurationSum != 1500*time.Millisecond {
		t.Fatalf("unexpected duration fields: %#v", snapshot)
	}
}

func TestCollectorWritesPrometheusText(t *testing.T) {
	t.Parallel()

	var collector Collector
	collector.JobStarted()
	collector.JobFinished(deploy.StatusFailed, time.Second)

	var out strings.Builder
	if err := collector.WritePrometheus(&out); err != nil {
		t.Fatalf("WritePrometheus() error = %v", err)
	}
	text := out.String()
	for _, want := range []string{
		"pk_deploy_jobs_started_total 1",
		`pk_deploy_jobs_completed_total{status="failed"} 1`,
		"pk_deploy_active_jobs 0",
		"pk_deploy_job_duration_seconds_sum 1.000000000",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Prometheus output missing %q:\n%s", want, text)
		}
	}
}
