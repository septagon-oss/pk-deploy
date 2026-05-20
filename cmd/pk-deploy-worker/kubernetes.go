package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/septagon-oss/pk-deploy/pkg/deploy"
	"github.com/septagon-oss/pk-deploy/pkg/worker"
)

const (
	serviceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	serviceAccountCAPath    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

func kubernetesInventoryExecutor(ctx context.Context, req worker.ExecuteRequest) (deploy.StepResult, error) {
	client, err := newKubernetesClient()
	if err != nil {
		return deploy.StepResult{}, err
	}
	namespaces := parseCSV(valueOr(req.Step.Inputs["namespaces"], os.Getenv("PK_DEPLOY_K8S_NAMESPACES")))
	if len(namespaces) == 0 {
		return deploy.StepResult{}, errors.New("kubernetes.inventory requires at least one namespace")
	}
	registry := registryClientFromEnv()
	observedAt := time.Now().UTC()
	environmentID := valueOr(req.Step.Inputs["environment"], req.Plan.Environment.ID)
	var components []deploy.ComponentState
	for _, namespace := range namespaces {
		items, err := client.listDeployments(ctx, namespace)
		if err != nil {
			return deploy.StepResult{}, err
		}
		for _, item := range items {
			components = append(components, item.components(ctx, registry, environmentID, observedAt)...)
		}
	}
	raw, err := json.Marshal(components)
	if err != nil {
		return deploy.StepResult{}, fmt.Errorf("encode components: %w", err)
	}
	return deploy.StepResult{
		Message: fmt.Sprintf("observed %d Kubernetes deployment components", len(components)),
		Outputs: map[string]string{
			"components": string(raw),
		},
	}, nil
}

func kubernetesSetImageExecutor(ctx context.Context, req worker.ExecuteRequest) (deploy.StepResult, error) {
	client, err := newKubernetesClient()
	if err != nil {
		return deploy.StepResult{}, err
	}
	namespace := strings.TrimSpace(req.Step.Inputs["namespace"])
	workload := strings.TrimSpace(req.Step.Inputs["workload"])
	container := strings.TrimSpace(req.Step.Inputs["container"])
	image := strings.TrimSpace(req.Step.Inputs["image"])
	if namespace == "" || workload == "" || container == "" || image == "" {
		return deploy.StepResult{}, errors.New("kubernetes.set-image requires namespace, workload, container, and image")
	}
	if err := client.patchDeploymentImage(ctx, namespace, workload, container, image); err != nil {
		return deploy.StepResult{}, err
	}
	return deploy.StepResult{
		Message: fmt.Sprintf("queued rollout for %s/%s container %s", namespace, workload, container),
		Outputs: map[string]string{
			"namespace": namespace,
			"workload":  workload,
			"container": container,
			"image":     image,
		},
	}, nil
}

type kubernetesClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func newKubernetesClient() (kubernetesClient, error) {
	host := strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_HOST"))
	port := valueOr(os.Getenv("KUBERNETES_SERVICE_PORT_HTTPS"), valueOr(os.Getenv("KUBERNETES_SERVICE_PORT"), "443"))
	if host == "" {
		return kubernetesClient{}, errors.New("KUBERNETES_SERVICE_HOST is required")
	}
	tokenBytes, err := os.ReadFile(valueOr(os.Getenv("PK_DEPLOY_K8S_TOKEN_FILE"), serviceAccountTokenPath))
	if err != nil {
		return kubernetesClient{}, fmt.Errorf("read Kubernetes service account token: %w", err)
	}
	caBytes, err := os.ReadFile(valueOr(os.Getenv("PK_DEPLOY_K8S_CA_FILE"), serviceAccountCAPath))
	if err != nil {
		return kubernetesClient{}, fmt.Errorf("read Kubernetes service account CA: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caBytes) {
		return kubernetesClient{}, errors.New("kubernetes service account CA did not contain certificates")
	}
	return kubernetesClient{
		baseURL: "https://" + host + ":" + port,
		token:   strings.TrimSpace(string(tokenBytes)),
		client: &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots},
		}},
	}, nil
}

func (c kubernetesClient) listDeployments(ctx context.Context, namespace string) ([]deploymentItem, error) {
	var list deploymentList
	path := "/apis/apps/v1/namespaces/" + url.PathEscape(namespace) + "/deployments"
	if err := c.doJSON(ctx, http.MethodGet, path, "", nil, &list); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c kubernetesClient) patchDeploymentImage(ctx context.Context, namespace, name, container, image string) error {
	patch := map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"containers": []map[string]string{{
						"name":  container,
						"image": image,
					}},
				},
			},
		},
	}
	path := "/apis/apps/v1/namespaces/" + url.PathEscape(namespace) + "/deployments/" + url.PathEscape(name)
	return c.doJSON(ctx, http.MethodPatch, path, "application/strategic-merge-patch+json", patch, nil)
}

func (c kubernetesClient) doJSON(ctx context.Context, method, path, contentType string, request, response any) error {
	var body io.Reader
	if request != nil {
		var encoded bytes.Buffer
		if err := json.NewEncoder(&encoded).Encode(request); err != nil {
			return fmt.Errorf("encode Kubernetes request: %w", err)
		}
		body = &encoded
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("kubernetes %s %s returned %s: %s", method, path, resp.Status, strings.TrimSpace(string(body)))
	}
	if response == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(response)
}

type deploymentList struct {
	Items []deploymentItem `json:"items"`
}

type deploymentItem struct {
	Metadata struct {
		Name      string            `json:"name"`
		Namespace string            `json:"namespace"`
		Labels    map[string]string `json:"labels"`
	} `json:"metadata"`
	Spec struct {
		Replicas *int `json:"replicas"`
		Template struct {
			Spec struct {
				Containers []containerSpec `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
	Status struct {
		ReadyReplicas int `json:"readyReplicas"`
	} `json:"status"`
}

type containerSpec struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

func (d deploymentItem) components(ctx context.Context, registry registryClient, environmentID string, observedAt time.Time) []deploy.ComponentState {
	desiredReplicas := 1
	if d.Spec.Replicas != nil {
		desiredReplicas = *d.Spec.Replicas
	}
	status := deploy.StatusRunning
	if desiredReplicas > 0 && d.Status.ReadyReplicas >= desiredReplicas {
		status = deploy.StatusSucceeded
	}
	out := make([]deploy.ComponentState, 0, len(d.Spec.Template.Spec.Containers))
	for _, container := range d.Spec.Template.Spec.Containers {
		ref := parseImageReference(container.Image)
		latestVersion, latestErr := registry.latestTag(ctx, ref)
		latestDigest := ""
		latestImage := ""
		updateAvailable := false
		labels := map[string]string{
			"imageRepository": ref.repository,
		}
		if latestErr == nil && latestVersion != "" {
			latestImage = ref.withTag(latestVersion)
			var digestErr error
			latestDigest, digestErr = registry.manifestDigest(ctx, ref, latestVersion)
			if digestErr != nil {
				labels["latestDigestResolution"] = digestErr.Error()
			}
			updateAvailable = latestDigest != "" && ref.tag != "" && ref.tag != latestVersion
		} else if latestErr != nil {
			labels["latestResolution"] = latestErr.Error()
		}
		id := d.Metadata.Namespace + "/" + d.Metadata.Name + "/" + container.Name
		out = append(out, deploy.ComponentState{
			ID:              id,
			Name:            d.Metadata.Name,
			EnvironmentID:   environmentID,
			Namespace:       d.Metadata.Namespace,
			WorkloadKind:    "Deployment",
			WorkloadName:    d.Metadata.Name,
			Container:       container.Name,
			Runtime:         "kubernetes",
			CurrentImage:    container.Image,
			CurrentVersion:  ref.version(),
			CurrentDigest:   ref.digest,
			LatestImage:     latestImage,
			LatestVersion:   latestVersion,
			LatestDigest:    latestDigest,
			ReadyReplicas:   d.Status.ReadyReplicas,
			DesiredReplicas: desiredReplicas,
			Status:          status,
			UpdateAvailable: updateAvailable,
			LastObservedAt:  observedAt,
			Labels:          labels,
		})
	}
	return out
}

type imageReference struct {
	registry   string
	repository string
	tag        string
	digest     string
}

func parseImageReference(raw string) imageReference {
	withoutDigest, digest, hasDigest := strings.Cut(strings.TrimSpace(raw), "@")
	lastSlash := strings.LastIndex(withoutDigest, "/")
	lastPart := withoutDigest[lastSlash+1:]
	pathWithoutTag := withoutDigest
	tag := ""
	if tagSep := strings.LastIndex(lastPart, ":"); tagSep >= 0 {
		tag = lastPart[tagSep+1:]
		pathWithoutTag = withoutDigest[:lastSlash+1] + lastPart[:tagSep]
	}
	first, rest, hasSlash := strings.Cut(pathWithoutTag, "/")
	ref := imageReference{tag: tag}
	if hasDigest {
		ref.digest = digest
	}
	if hasSlash && (strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost") {
		ref.registry = first
		ref.repository = rest
		return ref
	}
	ref.registry = "registry-1.docker.io"
	ref.repository = pathWithoutTag
	return ref
}

func (r imageReference) version() string {
	if r.tag != "" {
		return r.tag
	}
	if r.digest != "" {
		return r.digest
	}
	return "unknown"
}

func (r imageReference) withTag(tag string) string {
	if r.registry == "" {
		return r.repository + ":" + tag
	}
	return r.registry + "/" + r.repository + ":" + tag
}

type registryClient struct {
	scheme   string
	username string
	password string
	client   *http.Client
}

func registryClientFromEnv() registryClient {
	return registryClient{
		scheme:   valueOr(os.Getenv("PK_DEPLOY_REGISTRY_SCHEME"), "https"),
		username: strings.TrimSpace(os.Getenv("PK_DEPLOY_REGISTRY_USERNAME")),
		password: strings.TrimSpace(os.Getenv("PK_DEPLOY_REGISTRY_PASSWORD")),
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (c registryClient) latestTag(ctx context.Context, ref imageReference) (string, error) {
	if ref.registry == "" || ref.repository == "" {
		return "", errors.New("image reference does not include a registry repository")
	}
	endpoint := c.scheme + "://" + ref.registry + "/v2/" + ref.repository + "/tags/list"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	if c.username != "" || c.password != "" {
		httpReq.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return "", fmt.Errorf("registry tags returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var tags struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return "", err
	}
	return latestTag(tags.Tags, ref.tag)
}

func (c registryClient) manifestDigest(ctx context.Context, ref imageReference, tag string) (string, error) {
	if ref.registry == "" || ref.repository == "" || tag == "" {
		return "", errors.New("image reference, repository, and tag are required")
	}
	endpoint := c.scheme + "://" + ref.registry + "/v2/" + ref.repository + "/manifests/" + url.PathEscape(tag)
	digest, err := c.manifestDigestRequest(ctx, http.MethodHead, endpoint)
	if err == nil || !errors.Is(err, errManifestDigestMissing) {
		return digest, err
	}
	return c.manifestDigestRequest(ctx, http.MethodGet, endpoint)
}

var errManifestDigestMissing = errors.New("registry manifest response did not include Docker-Content-Digest")

func (c registryClient) manifestDigestRequest(ctx context.Context, method, endpoint string) (string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", "))
	if c.username != "" || c.password != "" {
		httpReq.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return "", fmt.Errorf("registry manifest returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	digest := strings.TrimSpace(resp.Header.Get("Docker-Content-Digest"))
	if digest == "" {
		return "", errManifestDigestMissing
	}
	return digest, nil
}

func (c registryClient) httpClient() *http.Client {
	if c.client != nil {
		return c.client
	}
	return &http.Client{Timeout: 10 * time.Second}
}

var semverPrefix = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)`)
var releaseSequence = regexp.MustCompile(`(?:^|[-_.])([0-9]{14})(?:$|[-_.])`)

func latestTag(tags []string, currentTag string) (string, error) {
	if len(tags) == 0 {
		return "", errors.New("registry returned no tags")
	}
	candidates := make([]tagCandidate, 0, len(tags))
	for _, tag := range tags {
		candidates = append(candidates, parseTagCandidate(tag))
	}
	slices.SortFunc(candidates, compareTagCandidate)
	best := candidates[len(candidates)-1]
	if currentTag != "" {
		current := parseTagCandidate(currentTag)
		if current.hasReleaseSequence() && sameTagRank(current, best) {
			return currentTag, nil
		}
	}
	return best.raw, nil
}

type tagCandidate struct {
	raw          string
	semver       bool
	major, minor int
	patch        int
	sequence     string
}

func parseTagCandidate(tag string) tagCandidate {
	candidate := tagCandidate{raw: tag}
	match := semverPrefix.FindStringSubmatch(tag)
	if len(match) != 4 {
		return candidate
	}
	candidate.semver = true
	candidate.major, _ = strconv.Atoi(match[1])
	candidate.minor, _ = strconv.Atoi(match[2])
	candidate.patch, _ = strconv.Atoi(match[3])
	if sequence := releaseSequence.FindStringSubmatch(tag); len(sequence) == 2 {
		candidate.sequence = sequence[1]
	}
	return candidate
}

func (c tagCandidate) hasReleaseSequence() bool {
	return c.semver && c.sequence != ""
}

func compareTagCandidate(a, b tagCandidate) int {
	if a.semver != b.semver {
		if a.semver {
			return 1
		}
		return -1
	}
	for _, pair := range [][2]int{{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if a.sequence < b.sequence {
		return -1
	}
	if a.sequence > b.sequence {
		return 1
	}
	if a.raw < b.raw {
		return -1
	}
	if a.raw > b.raw {
		return 1
	}
	return 0
}

func sameTagRank(a, b tagCandidate) bool {
	return a.semver == b.semver &&
		a.major == b.major &&
		a.minor == b.minor &&
		a.patch == b.patch &&
		a.sequence == b.sequence
}
