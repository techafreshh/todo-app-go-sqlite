package main

import (
	"os"
	"strings"
	"testing"
)

// README.md is the only instructions a fresh clone gets, and it re-declares the
// run interface main.go and compose.yaml own. This test binds the documented
// commands and env keys to those owners so a stale port, a renamed command, or
// a dropped configuration variable fails here instead of stranding a new user
// (PRD §9, §11, §12 Phase 4).
func TestReadmeDocumentsRunPaths(t *testing.T) {
	b, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	readme := string(b)

	for _, want := range []string{
		"go run .",
		"docker compose up --build",
		"http://localhost:" + defaultPort,
		envPort,
		envDBPath,
	} {
		if !strings.Contains(readme, want) {
			t.Errorf("README.md does not document %q", want)
		}
	}
}
