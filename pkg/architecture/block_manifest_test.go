package architecture_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

type blockManifest struct {
	SchemaVersion string          `json:"schemaVersion"`
	Repository    string          `json:"repository"`
	Blocks        []manifestBlock `json:"blocks"`
}

type manifestBlock struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	Owner           string   `json:"owner"`
	Version         string   `json:"version"`
	Package         string   `json:"package"`
	Status          string   `json:"status"`
	Contracts       []string `json:"contracts"`
	CompositionLaws []string `json:"compositionLaws"`
	ExtensionPoints []string `json:"extensionPoints"`
	Evidence        []string `json:"evidence"`
}

func TestDeployBlockManifestIsReleaseGrade(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	manifest := readBlockManifest(t, filepath.Join(repoRoot, "docs", "block-manifest.json"))
	if manifest.SchemaVersion != "pk.block-manifest.v1" {
		t.Fatalf("schemaVersion = %q", manifest.SchemaVersion)
	}
	if manifest.Repository != "pk-deploy" {
		t.Fatalf("repository = %q", manifest.Repository)
	}
	if len(manifest.Blocks) == 0 {
		t.Fatal("manifest must declare public blocks")
	}

	seen := map[string]struct{}{}
	for _, block := range manifest.Blocks {
		requireNonEmpty(t, block.ID, "id", block)
		requireNonEmpty(t, block.Kind, "kind", block)
		requireNonEmpty(t, block.Owner, "owner", block)
		requireNonEmpty(t, block.Version, "version", block)
		requireNonEmpty(t, block.Package, "package", block)
		if block.Status != "composable" {
			t.Fatalf("%s status = %q, want composable", block.ID, block.Status)
		}
		if _, exists := seen[block.ID]; exists {
			t.Fatalf("duplicate block id %q", block.ID)
		}
		seen[block.ID] = struct{}{}
		if len(block.Contracts) == 0 {
			t.Fatalf("%s must declare contracts", block.ID)
		}
		if len(block.CompositionLaws) == 0 {
			t.Fatalf("%s must declare composition laws", block.ID)
		}
		if len(block.ExtensionPoints) == 0 {
			t.Fatalf("%s must declare extension points", block.ID)
		}
		for _, evidence := range block.Evidence {
			if _, err := os.Stat(filepath.Join(repoRoot, evidence)); err != nil {
				t.Fatalf("%s evidence %q: %v", block.ID, evidence, err)
			}
		}
	}

	requireLaws(t, manifest, "pk-deploy.plan", "identity", "closure", "determinism", "substitution", "evidence")
	requireLaws(t, manifest, "pk-deploy.job", "identity", "determinism", "least privilege")
	requireLaws(t, manifest, "pk-deploy.worker", "closure", "substitution", "least privilege", "evidence")
	requireLaws(t, manifest, "pk-deploy.metrics", "determinism", "substitution")
}

func readBlockManifest(t *testing.T, path string) blockManifest {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read block manifest: %v", err)
	}
	var manifest blockManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode block manifest: %v", err)
	}
	return manifest
}

func requireNonEmpty(t *testing.T, value, name string, block manifestBlock) {
	t.Helper()
	if value == "" {
		t.Fatalf("%s must declare %s", block.ID, name)
	}
}

func requireLaws(t *testing.T, manifest blockManifest, id string, laws ...string) {
	t.Helper()
	block := manifestBlock{}
	for _, candidate := range manifest.Blocks {
		if candidate.ID == id {
			block = candidate
			break
		}
	}
	if block.ID == "" {
		t.Fatalf("missing block %q", id)
	}
	for _, law := range laws {
		if !slices.Contains(block.CompositionLaws, law) {
			t.Fatalf("%s missing composition law %q", id, law)
		}
	}
}
