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

	for _, want := range []string{
		"permissions:",          // an explicit permissions block exists
		"contents: read",        // read-only token
		"pull_request:",         // PRs are checked, not just main
		"go-version-file: go.mod", // toolchain derived from go.mod, not hand-copied
		"go vet ./...",
		"go test ./...",
		"go build ./...",
		"docker build",          // the shipped image builds in CI
	} {
		if !strings.Contains(wf, want) {
			t.Errorf("ci.yml missing required element %q", want)
		}
	}

	if strings.Contains(wf, "secrets.") {
		t.Error("ci.yml must not reference secrets")
	}
}
