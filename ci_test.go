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
	lines := composeLines(wf)

	// Required elements: the checks the merge queue relies on actually run.
	for _, want := range []string{
		"go-version-file: go.mod", // toolchain derived from go.mod, not hand-copied
		"go vet ./...",
		"go test ./...",
		"go build ./...",
		"docker build",  // the shipped image builds in CI
		"pull_request:", // every PR is checked
	} {
		if !strings.Contains(wf, want) {
			t.Errorf("ci.yml missing required element %q", want)
		}
	}

	// The workflow runs on push to main.
	push := composeBlock(t, composeBlock(t, lines, "on:", 0), "push:", 2)
	if !containsText(push, "main") {
		t.Error("ci.yml must run on push to main")
	}

	// No secret access and no template expressions anywhere in the workflow.
	for _, banned := range []string{"secrets", "${{"} {
		if strings.Contains(wf, banned) {
			t.Errorf("ci.yml must not contain %q", banned)
		}
	}

	// Exactly one permissions block in the whole file — a job-level override is
	// a second block and the effective token for that job — granting exactly
	// contents: read.
	perms := composeBlock(t, lines, "permissions:", 0)
	if len(perms) != 1 || perms[0].text != "contents: read" {
		t.Errorf("workflow permissions must grant exactly 'contents: read', got %v", perms)
	}
	if n := countHeader(lines, "permissions:"); n != 1 {
		t.Errorf("ci.yml must declare exactly one permissions block, found %d", n)
	}
}

func containsText(lines []yamlLine, want string) bool {
	for _, l := range lines {
		if strings.Contains(l.text, want) {
			return true
		}
	}
	return false
}

func countHeader(lines []yamlLine, header string) int {
	n := 0
	for _, l := range lines {
		if l.text == header {
			n++
		}
	}
	return n
}
