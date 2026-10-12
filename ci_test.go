package main

import (
	"os"
	"strings"
	"testing"
)

// TestCIWorkflowContract pins the security-relevant shape of the CI workflow
// that the merge queue hard-gates on. If this test fails after editing
// .github/workflows/ci.yml, change both sides together.
func TestCIWorkflowContract(t *testing.T) {
	raw, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("read ci.yml: %v", err)
	}
	wf := string(raw)

	// Required elements: the checks the merge queue relies on actually run.
	for _, want := range []string{
		"go-version-file: go.mod", // toolchain derived from go.mod, not hand-copied
		"go vet ./...",
		"go test ./...",
		"go build ./...",
		"docker build", // the shipped image builds in CI
		"branches: [main]",
	} {
		if !strings.Contains(wf, want) {
			t.Errorf("ci.yml missing required element %q", want)
		}
	}

	// No secret access and no template expressions anywhere in the workflow.
	for _, banned := range []string{"secrets", "${{", "id-token", "packages"} {
		if strings.Contains(wf, banned) {
			t.Errorf("ci.yml must not contain %q", banned)
		}
	}

	// The permissions block grants exactly contents: read — nothing else.
	body, ok := permissionsBlock(wf)
	if !ok {
		t.Fatal("ci.yml has no parseable permissions block")
	}
	if strings.TrimSpace(body) != "contents: read" {
		t.Errorf("permissions block must grant exactly 'contents: read', got %q", body)
	}
}

// permissionsBlock returns the indented body under a top-level "permissions:"
// key and whether one was found.
func permissionsBlock(wf string) (string, bool) {
	lines := strings.Split(wf, "\n")
	for i, line := range lines {
		if line != "permissions:" {
			continue
		}
		var body []string
		for _, next := range lines[i+1:] {
			if next == "" {
				continue
			}
			if !strings.HasPrefix(next, " ") {
				break
			}
			body = append(body, next)
		}
		return strings.Join(body, "\n"), true
	}
	return "", false
}
