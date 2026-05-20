package controlplane

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/septagon-oss/pk-deploy/pkg/deploy"
	"github.com/septagon-oss/pk-deploy/pkg/job"
	"github.com/septagon-oss/pk-deploy/pkg/worker"
)

// Server exposes the self-hosted control-plane HTTP API.
type Server struct {
	cfg   Config
	store *Store
	mux   *http.ServeMux
	now   func() time.Time
}

// NewServer returns a configured control-plane server.
func NewServer(cfg Config, store *Store) (*Server, error) {
	if len(cfg.Secret) < 32 {
		return nil, errors.New("shared secret must be at least 32 bytes")
	}
	if cfg.AdminToken == "" {
		return nil, errors.New("admin token is required")
	}
	if store == nil {
		store = NewStore(time.Now().UTC())
	}
	s := &Server{
		cfg:   cfg,
		store: store,
		mux:   http.NewServeMux(),
		now:   func() time.Time { return time.Now().UTC() },
	}
	s.routes()
	return s, nil
}

// Handler returns the server HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /", s.handleIndex)
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /metrics", s.handleMetrics)
	s.mux.HandleFunc("GET /api/status", s.handleStatus)
	s.mux.HandleFunc("POST /api/jobs", s.handleCreateJob)
	s.mux.HandleFunc("POST /api/jobs/inventory", s.handleInventoryJob)
	s.mux.HandleFunc("POST /api/jobs/sample", s.handleSampleJob)
	s.mux.HandleFunc("POST /api/components/update", s.handleComponentUpdate)
	s.mux.HandleFunc("POST /api/claim", s.handleClaim)
	s.mux.HandleFunc("POST /api/complete", s.handleComplete)
}

func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := indexTemplate.Execute(w, s.store.Snapshot()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (*Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}` + "\n"))
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(s.store.Metrics()))
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.store.Snapshot(), http.StatusOK)
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	defer func() { _ = r.Body.Close() }()
	var raw job.Job
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, fmt.Sprintf("decode job: %v", err), http.StatusBadRequest)
		return
	}
	signed, err := job.Sign(raw, s.cfg.KeyID, s.cfg.Secret)
	if err != nil {
		http.Error(w, fmt.Sprintf("sign job: %v", err), http.StatusBadRequest)
		return
	}
	if err := s.store.Enqueue(signed); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, signed, http.StatusCreated)
}

func (s *Server) handleSampleJob(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	now := s.now()
	signed, err := job.Sign(sampleJob(now), s.cfg.KeyID, s.cfg.Secret)
	if err != nil {
		http.Error(w, fmt.Sprintf("sign sample job: %v", err), http.StatusInternalServerError)
		return
	}
	if err := s.store.Enqueue(signed); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, signed, http.StatusCreated)
}

func (s *Server) handleInventoryJob(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	now := s.now()
	signed, err := job.Sign(s.inventoryJob(now), s.cfg.KeyID, s.cfg.Secret)
	if err != nil {
		http.Error(w, fmt.Sprintf("sign inventory job: %v", err), http.StatusInternalServerError)
		return
	}
	if err := s.store.Enqueue(signed); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, signed, http.StatusCreated)
}

func (s *Server) handleComponentUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	defer func() { _ = r.Body.Close() }()
	var request componentUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("decode update request: %v", err), http.StatusBadRequest)
		return
	}
	component, ok := s.store.Component(strings.TrimSpace(request.ComponentID))
	if !ok {
		http.Error(w, "component has not been observed yet; refresh inventory first", http.StatusNotFound)
		return
	}
	if !component.UpdateAvailable {
		http.Error(w, "component is already on the latest observed version", http.StatusConflict)
		return
	}
	if strings.TrimSpace(component.LatestImage) == "" {
		http.Error(w, "component latest image is unknown", http.StatusConflict)
		return
	}
	if !strings.Contains(component.LatestDigest, ":") {
		http.Error(w, "component latest digest is unknown; refresh inventory and retry", http.StatusConflict)
		return
	}
	now := s.now()
	signed, err := job.Sign(updateComponentJob(component, now), s.cfg.KeyID, s.cfg.Secret)
	if err != nil {
		http.Error(w, fmt.Sprintf("sign update job: %v", err), http.StatusInternalServerError)
		return
	}
	if err := s.store.Enqueue(signed); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, signed, http.StatusCreated)
}

func (s *Server) handleClaim(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	var info worker.Info
	if err := json.NewDecoder(r.Body).Decode(&info); err != nil {
		http.Error(w, fmt.Sprintf("decode worker info: %v", err), http.StatusBadRequest)
		return
	}
	signed, err := s.store.Claim(r.Context(), info)
	if err != nil {
		if errors.Is(err, worker.ErrNoJob) {
			http.Error(w, "no job", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, signed, http.StatusOK)
}

func (s *Server) handleComplete(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	var result worker.Result
	if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
		http.Error(w, fmt.Sprintf("decode result: %v", err), http.StatusBadRequest)
		return
	}
	if err := s.store.Complete(r.Context(), result); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]bool{"ok": true}, http.StatusOK)
}

func (s *Server) authorized(r *http.Request) bool {
	got := r.Header.Get("X-PK-Deploy-Admin-Token")
	if got == "" {
		got = r.URL.Query().Get("token")
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.AdminToken)) == 1
}

func sampleJob(now time.Time) job.Job {
	return job.Job{
		ID: "sample-" + now.Format("20060102T150405Z"),
		Plan: deploy.Plan{
			ID: "sample-plan-" + now.Format("20060102T150405Z"),
			Application: deploy.Application{
				ID:   "pk-deploy-smoke",
				Name: "pk-deploy smoke",
			},
			Environment: deploy.Environment{
				ID:   "staging",
				Name: "Staging",
			},
			Artifacts: []deploy.Artifact{{
				ID:     "pk-deploy-runtime",
				Kind:   "container-image",
				Ref:    "pk-deploy:local",
				Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
			}},
			Gates: []deploy.Gate{{
				ID:       "operator-requested",
				Kind:     "manual",
				Required: true,
				Status:   deploy.StatusSucceeded,
			}},
			Steps: []deploy.Step{{
				ID:       "worker-roundtrip",
				Executor: "noop",
				Action:   "succeed",
				Inputs: map[string]string{
					"message": "worker reached the control plane and completed a signed job",
				},
			}},
			CreatedAt: now,
		},
		Selector: deploy.WorkerSelector{
			Capabilities: []string{"noop"},
			Labels:       map[string]string{"environment": "staging"},
		},
		IssuedAt:  now,
		ExpiresAt: now.Add(15 * time.Minute),
		Nonce:     "nonce-" + now.Format("20060102T150405.000000000Z"),
	}
}

func (s *Server) inventoryJob(now time.Time) job.Job {
	return job.Job{
		ID: "inventory-" + now.Format("20060102T150405Z"),
		Plan: deploy.Plan{
			ID: "inventory-plan-" + now.Format("20060102T150405Z"),
			Application: deploy.Application{
				ID:   inventoryApplicationID,
				Name: "Deployment inventory",
			},
			Environment: deploy.Environment{
				ID:   s.cfg.EnvironmentID,
				Name: s.cfg.EnvironmentID,
			},
			Steps: []deploy.Step{{
				ID:       "kubernetes-inventory",
				Executor: kubernetesInventoryExecutor,
				Action:   "list-deployments",
				Inputs: map[string]string{
					"environment": s.cfg.EnvironmentID,
					"namespaces":  strings.Join(s.cfg.InventoryNamespaces, ","),
				},
				TimeoutSeconds: 30,
			}},
			CreatedAt: now,
		},
		Selector: deploy.WorkerSelector{
			Capabilities: []string{kubernetesInventoryExecutor},
			Labels:       map[string]string{"environment": s.cfg.EnvironmentID},
		},
		IssuedAt:  now,
		ExpiresAt: now.Add(15 * time.Minute),
		Nonce:     "nonce-" + now.Format("20060102T150405.000000000Z"),
	}
}

func updateComponentJob(component deploy.ComponentState, now time.Time) job.Job {
	environmentID := valueOr(component.EnvironmentID, "unknown")
	return job.Job{
		ID: "update-" + slug(component.ID) + "-" + now.Format("20060102T150405Z"),
		Plan: deploy.Plan{
			ID: "update-plan-" + slug(component.ID) + "-" + now.Format("20060102T150405Z"),
			Application: deploy.Application{
				ID:   component.ID,
				Name: component.Name,
			},
			Environment: deploy.Environment{
				ID:   environmentID,
				Name: environmentID,
			},
			Artifacts: []deploy.Artifact{{
				ID:     component.ID,
				Kind:   "container-image",
				Ref:    component.LatestImage,
				Digest: component.LatestDigest,
			}},
			Steps: []deploy.Step{{
				ID:       "set-image",
				Executor: "kubernetes.set-image",
				Action:   "set-image",
				Inputs: map[string]string{
					"namespace": component.Namespace,
					"workload":  component.WorkloadName,
					"container": component.Container,
					"image":     component.LatestImage,
				},
				TimeoutSeconds: 30,
			}},
			CreatedAt: now,
		},
		Selector: deploy.WorkerSelector{
			Capabilities: []string{"kubernetes.set-image"},
			Labels:       map[string]string{"environment": environmentID},
		},
		IssuedAt:  now,
		ExpiresAt: now.Add(15 * time.Minute),
		Nonce:     "nonce-" + now.Format("20060102T150405.000000000Z"),
	}
}

type componentUpdateRequest struct {
	ComponentID string `json:"componentId"`
}

func slug(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "component"
	}
	return out
}

func writeJSON(w http.ResponseWriter, value any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

var indexTemplate = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>pk-deploy</title>
  <style>
    :root { color-scheme: light dark; font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    body { margin: 0; padding: 28px; background: Canvas; color: CanvasText; }
    main { max-width: 1360px; margin: 0 auto; }
    header { display: flex; justify-content: space-between; gap: 24px; align-items: start; margin-bottom: 24px; }
    h1 { font-size: 28px; line-height: 1.15; margin: 0 0 8px; }
    h2 { font-size: 17px; margin: 0 0 12px; }
    p { margin: 0; color: color-mix(in srgb, CanvasText 72%, Canvas); }
    section { border: 1px solid color-mix(in srgb, CanvasText 16%, Canvas); border-radius: 8px; padding: 18px; margin: 16px 0; }
    table { width: 100%; border-collapse: collapse; }
    th, td { text-align: left; padding: 10px 9px; border-bottom: 1px solid color-mix(in srgb, CanvasText 12%, Canvas); font-size: 14px; vertical-align: top; }
    th { color: color-mix(in srgb, CanvasText 70%, Canvas); font-size: 12px; text-transform: uppercase; letter-spacing: 0; }
    code { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 12px; }
    label { display: block; font-size: 13px; margin-bottom: 6px; }
    input { width: min(440px, 100%); padding: 9px 10px; border: 1px solid color-mix(in srgb, CanvasText 18%, Canvas); border-radius: 6px; background: Canvas; color: CanvasText; }
    button { padding: 9px 12px; border: 0; border-radius: 6px; background: #0f766e; color: white; font-weight: 650; cursor: pointer; white-space: nowrap; }
    button.secondary { background: color-mix(in srgb, CanvasText 12%, Canvas); color: CanvasText; }
    button:disabled { cursor: not-allowed; opacity: .55; }
    .summary { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 12px; }
    .metric { font-size: 28px; font-weight: 750; margin-top: 2px; }
    .actions { display: flex; gap: 8px; align-items: end; flex-wrap: wrap; }
    .status { min-height: 20px; font-size: 13px; margin-top: 8px; color: color-mix(in srgb, CanvasText 72%, Canvas); }
    .pill { display: inline-flex; align-items: center; min-height: 22px; padding: 2px 8px; border-radius: 999px; font-size: 12px; font-weight: 700; background: color-mix(in srgb, CanvasText 10%, Canvas); }
    .ok { background: color-mix(in srgb, #16a34a 22%, Canvas); color: color-mix(in srgb, #16a34a 80%, CanvasText); }
    .stale { background: color-mix(in srgb, #ca8a04 20%, Canvas); color: color-mix(in srgb, #ca8a04 76%, CanvasText); }
    .bad { background: color-mix(in srgb, #dc2626 20%, Canvas); color: color-mix(in srgb, #dc2626 78%, CanvasText); }
    .muted { color: color-mix(in srgb, CanvasText 58%, Canvas); }
    .image { max-width: 320px; overflow-wrap: anywhere; }
    .version { min-width: 150px; }
    @media (max-width: 900px) {
      body { padding: 16px; }
      header { display: block; }
      .summary { grid-template-columns: repeat(2, minmax(0, 1fr)); }
      .scroll { overflow-x: auto; }
      table { min-width: 960px; }
    }
  </style>
</head>
<body>
<main>
  <header>
    <div>
      <h1>pk-deploy</h1>
      <p>Deployment inventory, version drift, signed updates, evidence, and Prometheus metrics.</p>
    </div>
    <div><code>/healthz</code> <code>/metrics</code> <code>/api/status</code></div>
  </header>

  <section class="summary" aria-label="Summary">
    <div><h2>Components</h2><div class="metric">{{ len .Components }}</div></div>
    <div><h2>Ready</h2><div class="metric">{{ .ReadyComponents }}</div></div>
    <div><h2>Updates</h2><div class="metric">{{ .StaleComponents }}</div></div>
    <div><h2>Pending</h2><div class="metric">{{ len .Pending }}</div></div>
    <div><h2>Completed</h2><div class="metric">{{ len .Completed }}</div></div>
  </section>

  <section>
    <h2>Controls</h2>
    <div class="actions">
      <div>
        <label for="token">Admin token</label>
        <input id="token" type="password" autocomplete="off" placeholder="X-PK-Deploy-Admin-Token">
      </div>
      <button type="button" onclick="refreshInventory()">Refresh inventory</button>
      <button class="secondary" type="button" onclick="location.reload()">Reload</button>
    </div>
    <div class="status" id="status"></div>
  </section>

  <section>
    <h2>Deployed components</h2>
    <div class="scroll">
      <table>
        <thead><tr><th>Status</th><th>Component</th><th>Target</th><th>Replicas</th><th>Current</th><th>Latest</th><th>Action</th></tr></thead>
        <tbody>
        {{ range .Components }}
          <tr>
            <td>{{ if eq .Status "succeeded" }}<span class="pill ok">Ready</span>{{ else }}<span class="pill bad">{{ .Status }}</span>{{ end }}</td>
            <td><strong>{{ .Name }}</strong><br><code>{{ .ID }}</code></td>
            <td>{{ .EnvironmentID }}<br><span class="muted">{{ .Namespace }} / {{ .WorkloadName }} / {{ .Container }}</span></td>
            <td>{{ .ReadyReplicas }} / {{ .DesiredReplicas }}</td>
            <td class="version"><code>{{ if .CurrentVersion }}{{ .CurrentVersion }}{{ else }}{{ .CurrentImage }}{{ end }}</code><br><span class="muted image">{{ .CurrentImage }}</span></td>
            <td class="version">{{ if .UpdateAvailable }}<span class="pill stale">Update available</span>{{ else }}<span class="pill ok">Latest</span>{{ end }}<br><code>{{ if .LatestVersion }}{{ .LatestVersion }}{{ else }}unknown{{ end }}</code></td>
            <td>{{ if .UpdateAvailable }}<button type="button" data-component="{{ .ID }}" onclick="queueUpdate(this.dataset.component)">Update to latest</button>{{ else }}<button type="button" disabled>Current</button>{{ end }}</td>
          </tr>
        {{ else }}
          <tr><td colspan="7">No component inventory yet. Enter the admin token and refresh inventory.</td></tr>
        {{ end }}
        </tbody>
      </table>
    </div>
  </section>

  <section>
    <h2>Recent signed jobs</h2>
    <div class="scroll">
      <table>
        <thead><tr><th>ID</th><th>Plan</th><th>Status</th><th>Message</th><th>Finished</th></tr></thead>
        <tbody>
        {{ range .Completed }}<tr><td><code>{{ .JobID }}</code></td><td><code>{{ .PlanID }}</code></td><td>{{ .Status }}</td><td>{{ .Message }}</td><td>{{ .FinishedAt }}</td></tr>{{ else }}<tr><td colspan="5">No completed jobs.</td></tr>{{ end }}
        </tbody>
      </table>
    </div>
  </section>
</main>
<script>
const tokenInput = document.getElementById('token');
const statusBox = document.getElementById('status');
tokenInput.value = localStorage.getItem('pkDeployAdminToken') || '';
tokenInput.addEventListener('input', () => localStorage.setItem('pkDeployAdminToken', tokenInput.value));
async function postJSON(path, body) {
  const headers = {'X-PK-Deploy-Admin-Token': tokenInput.value};
  if (body) headers['Content-Type'] = 'application/json';
  const res = await fetch(path, {method: 'POST', headers, body: body ? JSON.stringify(body) : undefined});
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}
async function refreshInventory() {
  try {
    statusBox.textContent = 'Inventory refresh queued. The cluster worker should report back shortly.';
    await postJSON('/api/jobs/inventory');
    setTimeout(() => location.reload(), 6500);
  } catch (error) {
    statusBox.textContent = error.message;
  }
}
async function queueUpdate(componentId) {
  try {
    statusBox.textContent = 'Update job queued for ' + componentId + '.';
    await postJSON('/api/components/update', {componentId});
    setTimeout(() => location.reload(), 6500);
  } catch (error) {
    statusBox.textContent = error.message;
  }
}
</script>
</body>
</html>`))
