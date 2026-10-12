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
		"docker build", // the shipped image builds in CI
	} {
		if !strings.Contains(wf, want) {
			t.Errorf("ci.yml missing required element %q", want)
		}
	}

	// Triggers, as parsed values: the workflow runs on push to main and on
	// every pull request. Exact parsed keys, not substrings — branches-ignore,
	// paths filters, or a commented-out trigger must fail this.
	onBlock := composeBlock(t, lines, "on:", 0)
	push := composeBlock(t, onBlock, "push:", 2)
	if got := composeScalar(t, push, "branches:", 4); got != "[main]" {
		t.Errorf("ci.yml must run on push to main via branches: [main], got %q", got)
	}
	composeBlock(t, onBlock, "pull_request:", 2)

	// No secret access, no template expressions, and no trigger forms that
	// change what the triggers mean.
	for _, banned := range []string{"secrets", "${{", "pull_request_target"} {
		if strings.Contains(wf, banned) {
			t.Errorf("ci.yml must not contain %q", banned)
		}
	}

	// Exactly one permissions declaration in the whole file — every line
	// prefixed "permissions:" counts, so inline, scalar, and job-level
	// overrides all fail — and it must be the top-level block granting
	// exactly contents: read.
	n := 0
	for _, l := range lines {
		if strings.HasPrefix(l.text, "permissions:") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("ci.yml must declare exactly one permissions declaration (a job-level or inline override is a second), found %d", n)
	}
	perms := composeBlock(t, lines, "permissions:", 0)
	if len(perms) != 1 || perms[0].text != "contents: read" {
		t.Errorf("workflow permissions must grant exactly 'contents: read', got %v", perms)
	}
}
