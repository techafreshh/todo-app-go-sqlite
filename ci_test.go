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
	// every pull request. Exact lines, not substrings — branches-ignore,
	// paths filters, or a commented-out trigger must fail this.
	onBlock := composeBlock(t, lines, "on:", 0)
	push := composeBlock(t, onBlock, "push:", 2)
	if !hasExactLine(push, "branches: [main]") {
		t.Error("ci.yml must run on push to main via an exact 'branches: [main]' entry")
	}
	composeBlock(t, onBlock, "pull_request:", 2)

	// No secret access, no template expressions, and no trigger forms that
	// change what the triggers mean.
	for _, banned := range []string{"secrets", "${{", "pull_request_target"} {
		if strings.Contains(wf, banned) {
			t.Errorf("ci.yml must not contain %q", banned)
		}
	}

	// One permissions declaration in the whole file — the top-level block
	// header, nothing inline and no job-level override — granting exactly
	// contents: read.
	perms := composeBlock(t, lines, "permissions:", 0)
	if len(perms) != 1 || perms[0].text != "contents: read" {
		t.Errorf("workflow permissions must grant exactly 'contents: read', got %v", perms)
	}
	for _, l := range lines {
		if strings.HasPrefix(l.text, "permissions:") && l.text != "permissions:" {
			t.Errorf("ci.yml must not declare an inline or job-level permissions override, found %q", l.text)
		}
	}
}

func hasExactLine(lines []yamlLine, want string) bool {
	for _, l := range lines {
		if l.text == want {
			return true
		}
	}
	return false
}
