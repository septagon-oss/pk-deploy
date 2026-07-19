// Implements: REQ-INFRA-006.
// Per: ADR-0029.
// Discipline: C-14.

// Package metrics exposes deployment worker health without forcing a metrics SDK.
package metrics

import (
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/septagon-oss/pk-deploy/pkg/deploy"
)

// Collector records worker counters and exposes Prometheus text format.
type Collector struct {
	started       atomic.Uint64
	succeeded     atomic.Uint64
	failed        atomic.Uint64
	active        atomic.Int64
	durationCount atomic.Uint64
	durationNanos atomic.Uint64
}

// Snapshot is an immutable view of collector state.
type Snapshot struct {
	Started       uint64
	Succeeded     uint64
	Failed        uint64
	Active        int64
	DurationCount uint64
	DurationSum   time.Duration
}

// JobStarted records a claimed job.
func (c *Collector) JobStarted() {
	if c == nil {
		return
	}
	c.started.Add(1)
	c.active.Add(1)
}

// JobFinished records a completed job.
func (c *Collector) JobFinished(status deploy.Status, duration time.Duration) {
	if c == nil {
		return
	}
	if status == deploy.StatusSucceeded {
		c.succeeded.Add(1)
	} else {
		c.failed.Add(1)
	}
	c.decrementActive()
	if duration < 0 {
		duration = 0
	}
	c.durationCount.Add(1)
	c.durationNanos.Add(uint64(duration))
}

func (c *Collector) decrementActive() {
	for {
		current := c.active.Load()
		if current <= 0 {
			return
		}
		if c.active.CompareAndSwap(current, current-1) {
			return
		}
	}
}

// Snapshot returns the current collector state.
func (c *Collector) Snapshot() Snapshot {
	if c == nil {
		return Snapshot{}
	}
	return Snapshot{
		Started:       c.started.Load(),
		Succeeded:     c.succeeded.Load(),
		Failed:        c.failed.Load(),
		Active:        c.active.Load(),
		DurationCount: c.durationCount.Load(),
		DurationSum:   time.Duration(c.durationNanos.Load()),
	}
}

// ServeHTTP writes Prometheus text exposition.
func (c *Collector) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_ = c.WritePrometheus(w)
}

// WritePrometheus writes Prometheus text exposition.
func (c *Collector) WritePrometheus(w io.Writer) error {
	s := c.Snapshot()
	if _, err := fmt.Fprintln(w, "# HELP pk_deploy_jobs_started_total Deployment jobs claimed by this worker."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# TYPE pk_deploy_jobs_started_total counter"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "pk_deploy_jobs_started_total %d\n", s.Started); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# HELP pk_deploy_jobs_completed_total Deployment jobs completed by normalized status."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# TYPE pk_deploy_jobs_completed_total counter"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "pk_deploy_jobs_completed_total{status=\"succeeded\"} %d\n", s.Succeeded); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "pk_deploy_jobs_completed_total{status=\"failed\"} %d\n", s.Failed); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# HELP pk_deploy_active_jobs Deployment jobs currently executing."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# TYPE pk_deploy_active_jobs gauge"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "pk_deploy_active_jobs %d\n", s.Active); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# HELP pk_deploy_job_duration_seconds Deployment job duration summary."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# TYPE pk_deploy_job_duration_seconds summary"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "pk_deploy_job_duration_seconds_count %d\n", s.DurationCount); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "pk_deploy_job_duration_seconds_sum %.9f\n", s.DurationSum.Seconds())
	return err
}
