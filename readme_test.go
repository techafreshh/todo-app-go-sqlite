package main

import (
	"os"
	"strings"
	"testing"
)

// README.md is the only instructions a fresh clone gets, and it re-declares the
// run interface and API surface main.go, compose.yaml, and go.mod own. This test
// binds each documented value to its owner so a stale port, path, version, or
// route fails here instead of stranding or misdirecting a new user (PRD §9, §11,
// §12 Phase 4).
func TestReadmeDocumentsRunPaths(t *testing.T) {
	b, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	readme := string(b)

	// The two run commands and the env keys main.go reads.
	for _, want := range []string{
		"go run .",
		"docker compose up --build",
		envPort,
		envDBPath,
	} {
		if !strings.Contains(readme, want) {
			t.Errorf("README.md does not document %q", want)
		}
	}

	// The local run defaults main.go owns.
	if !strings.Contains(readme, defaultDBPath) {
		t.Errorf("README.md does not document the local DB path %q", defaultDBPath)
	}
	if url := "http://localhost:" + defaultPort; !strings.Contains(readme, url) {
		t.Errorf("README.md does not document the Go default URL %q", url)
	}

	// The Docker runtime contract compose.yaml declares: the DB path and the
	// published port behind the documented UI URL. The compose and Dockerfile
	// tests already hold those two files consistent.
	lines := composeLines(readCompose(t))
	todo := composeBlock(t, composeBlock(t, lines, "services:", 0), "todo:", 2)
	envs := composeEnv(t, composeBlock(t, todo, "environment:", 4))
	if dockerDBPath := envs[envDBPath]; !strings.Contains(readme, dockerDBPath) {
		t.Errorf("README.md does not document the Docker DB path %q", dockerDBPath)
	}
	published := strings.SplitN(composeList(t, composeBlock(t, todo, "ports:", 4))[0], ":", 2)[0]
	if url := "http://localhost:" + published; !strings.Contains(readme, url) {
		t.Errorf("README.md does not document the published URL %q", url)
	}

	// The Go version go.mod requires, which the README restates.
	if version := goVersionMajorMinor(t); !strings.Contains(readme, version) {
		t.Errorf("README.md does not document the Go version %q", version)
	}

	// Every declared route, bound to the README's API table.
	for _, route := range []string{
		routeHealthz, routeCreateTodo, routeListTodos, routeUpdateTodo,
		routeToggleTodo, routeDeleteTodo, routeIndex,
	} {
		method, path := splitRoute(t, route)
		// The mux's "{$}" anchor only ends the root pattern; the documented
		// path is the user-facing "/".
		path = strings.TrimSuffix(path, "{$}")
		if !strings.Contains(readme, "`"+method+"`") {
			t.Errorf("README.md API table does not document method %q", method)
		}
		if !strings.Contains(readme, "`"+path+"`") {
			t.Errorf("README.md API table does not document path %q", path)
		}
	}
}

// goVersionMajorMinor returns the "major.minor" of the go directive in go.mod,
// the form the README's requirements section states.
func goVersionMajorMinor(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "go" {
			parts := strings.SplitN(fields[1], ".", 3)
			if len(parts) < 2 {
				t.Fatalf("go.mod go directive %q is not MAJOR.MINOR[.PATCH]", fields[1])
			}
			return parts[0] + "." + parts[1]
		}
	}
	t.Fatal("go.mod has no go directive")
	return ""
}
