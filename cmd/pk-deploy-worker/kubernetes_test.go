// Validates: REQ-INFRA-006.
// Per: ADR-0029.
// Discipline: C-14.

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseImageReferenceWithRegistryPortTag(t *testing.T) {
	t.Parallel()

	ref := parseImageReference("192.168.1.200:3000/atlantis/platformkit/complete-saas-microservices:0.1.19-design-parity-e2e7b34116e4")
	if ref.registry != "192.168.1.200:3000" {
		t.Fatalf("registry = %q", ref.registry)
	}
	if ref.repository != "atlantis/platformkit/complete-saas-microservices" {
		t.Fatalf("repository = %q", ref.repository)
	}
	if ref.tag != "0.1.19-design-parity-e2e7b34116e4" {
		t.Fatalf("tag = %q", ref.tag)
	}
	if got := ref.withTag("0.1.20"); got != "192.168.1.200:3000/atlantis/platformkit/complete-saas-microservices:0.1.20" {
		t.Fatalf("withTag() = %q", got)
	}
}

func TestLatestTagPrefersHighestSemverPrefix(t *testing.T) {
	t.Parallel()

	got, err := latestTag([]string{
		"0.1.9-48eb4c940a3a",
		"0.1.19-design-parity-6cde21baec05-manual",
		"0.1.19-design-parity-e2e7b34116e4",
		"0.1.18-velora-darkmode",
	}, "")
	if err != nil {
		t.Fatalf("latestTag() error = %v", err)
	}
	if got != "0.1.19-design-parity-e2e7b34116e4" {
		t.Fatalf("latestTag() = %q", got)
	}
}

func TestLatestTagUsesReleaseSequenceBeforeHashSuffix(t *testing.T) {
	t.Parallel()

	got, err := latestTag([]string{
		"0.1.28-staging-20260519140310-c132c3ac58c8",
		"0.1.28-staging-20260519140850-995737cdeec8",
		"0.1.28-staging-20260519140849-c86aa1307fb1",
	}, "")
	if err != nil {
		t.Fatalf("latestTag() error = %v", err)
	}
	if got != "0.1.28-staging-20260519140850-995737cdeec8" {
		t.Fatalf("latestTag() = %q", got)
	}
}

func TestLatestTagPreservesCurrentReleaseForEquivalentBuildIdentity(t *testing.T) {
	t.Parallel()

	current := "0.1.28-staging-20260519140850-995737cdeec8"
	got, err := latestTag([]string{
		current,
		"0.1.28-staging-20260519140850-c86aa1307fb1",
	}, current)
	if err != nil {
		t.Fatalf("latestTag() error = %v", err)
	}
	if got != current {
		t.Fatalf("latestTag() = %q", got)
	}
}

func TestLatestTagDoesNotPreserveOpaqueCurrentTag(t *testing.T) {
	t.Parallel()

	got, err := latestTag([]string{
		"staging-5354992e71c9",
		"staging-788a628c19e0",
	}, "staging-5354992e71c9")
	if err != nil {
		t.Fatalf("latestTag() error = %v", err)
	}
	if got != "staging-788a628c19e0" {
		t.Fatalf("latestTag() = %q", got)
	}
}

func TestRegistryClientResolvesLatestTagDigest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/acme/app/tags/list":
			_, _ = w.Write([]byte(`{"tags":["0.1.0","0.2.0"]}`))
		case "/v2/acme/app/manifests/0.2.0":
			w.Header().Set("Docker-Content-Digest", "sha256:1234567890abcdef")
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ref := imageReference{
		registry:   strings.TrimPrefix(server.URL, "http://"),
		repository: "acme/app",
		tag:        "0.1.0",
	}
	client := registryClient{scheme: "http", client: server.Client()}
	tag, err := client.latestTag(t.Context(), ref)
	if err != nil {
		t.Fatalf("latestTag() error = %v", err)
	}
	if tag != "0.2.0" {
		t.Fatalf("latestTag() = %q", tag)
	}
	digest, err := client.manifestDigest(t.Context(), ref, tag)
	if err != nil {
		t.Fatalf("manifestDigest() error = %v", err)
	}
	if digest != "sha256:1234567890abcdef" {
		t.Fatalf("manifestDigest() = %q", digest)
	}
}
