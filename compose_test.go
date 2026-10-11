package main

import (
	"os"
	"path"
	"strings"
	"testing"
)

// compose.yaml is the deployment contract that feeds the image's runtime
// interface into a running service. This test reads it and binds it to the env
// keys main.go owns and the DB path and volume the Dockerfile declares, so a
// rename, a retargeted mount, a non-persistent volume, or an added service or
// credential source fails here instead of silently launching the container with
// a default DB path, writing the database outside the named volume, or reaching
// an external service (PRD §8, §9, §14.5).
func TestComposeBindsRuntimeContract(t *testing.T) {
	lines := composeLines(readCompose(t))

	services := composeBlock(t, lines, "services:", 0)
	// The deployment is one self-contained service; a sibling service is either
	// an unbound second process or an external dependency (PRD §9).
	if names := composeChildKeys(services, 2); len(names) != 1 || names[0] != "todo:" {
		t.Errorf("compose services = %v, want exactly [todo:]", names)
	}
	todo := composeBlock(t, services, "todo:", 2)

	df := readDockerfile(t)
	imageEnvs := dockerfileEnv(t, df)

	envs := composeEnv(t, composeBlock(t, todo, "environment:", 4))
	for _, key := range []string{envPort, envDBPath} {
		if _, ok := envs[key]; !ok {
			t.Errorf("compose service does not set %s, which main.go reads", key)
		}
	}
	for key := range envs {
		if key != envPort && key != envDBPath {
			t.Errorf("compose sets env %s, which the server never reads", key)
		}
	}
	if envs[envDBPath] != imageEnvs[envDBPath] {
		t.Errorf("compose DB_PATH %s != image DB_PATH %s", envs[envDBPath], imageEnvs[envDBPath])
	}
	// A credential file, secret, or external config would inject state the
	// single-user, no-auth service must not read (PRD §9).
	for _, key := range []string{"env_file:", "secrets:", "configs:"} {
		if composeHasKey(todo, key, 4) {
			t.Errorf("compose service todo declares %s, which injects credentials or external config", key)
		}
	}

	// The published port must map the port the server actually listens on.
	// main.go falls back to defaultPort when PORT is unset or empty, so an
	// empty PORT with a mismatched mapping would leave the service unreachable.
	listenPort := envs[envPort]
	if listenPort == "" {
		listenPort = defaultPort
	}
	ports := composeList(t, composeBlock(t, todo, "ports:", 4))
	if want := listenPort + ":" + listenPort; len(ports) != 1 || ports[0] != want {
		t.Errorf("compose ports = %v, want [%s]", ports, want)
	}

	// The DB directory must be mounted from the named volume, never a bind
	// mount or an image-layer path, or writes are lost on down/up.
	dbDir := path.Dir(envs[envDBPath])
	mounts := composeList(t, composeBlock(t, todo, "volumes:", 4))
	if want := "todo-data:" + dbDir; len(mounts) != 1 || mounts[0] != want {
		t.Errorf("compose service volumes = %v, want [%s]", mounts, want)
	}

	if got := composeScalar(t, todo, "restart:", 4); got != "unless-stopped" {
		t.Errorf("compose restart = %q, want unless-stopped", got)
	}
	if got := composeScalar(t, todo, "build:", 4); got != "." {
		t.Errorf("compose build = %q, want . (the existing Dockerfile)", got)
	}

	// The named volume must be backed by the persistent local driver; a tmpfs
	// or other driver drops the DB on down/up (PRD §14.5).
	vol := composeBlock(t, lines, "volumes:", 0)
	if !composeHasKey(vol, "todo-data:", 2) {
		t.Fatal("compose does not declare the top-level named volume todo-data")
	}
	for _, l := range composeBlock(t, vol, "todo-data:", 2) {
		if strings.Contains(l.text, "tmpfs") {
			t.Errorf("compose volume todo-data uses tmpfs, which does not persist: %q", l.text)
		}
		if l.indent == 4 && strings.HasPrefix(l.text, "driver:") {
			if d := strings.TrimSpace(strings.TrimPrefix(l.text, "driver:")); d != "local" {
				t.Errorf("compose volume todo-data driver = %q, want local (persistent)", d)
			}
		}
	}
}

// yamlLine is one significant line of compose.yaml with its leading-space
// indentation, which delimits the block structure the helpers below walk.
type yamlLine struct {
	indent int
	text   string
}

func readCompose(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatalf("read compose.yaml: %v", err)
	}
	return string(b)
}

func composeLines(s string) []yamlLine {
	var out []yamlLine
	for _, raw := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		trimmed := strings.TrimRight(raw, " \t")
		if strings.TrimSpace(trimmed) == "" {
			continue
		}
		indent := len(trimmed) - len(strings.TrimLeft(trimmed, " "))
		out = append(out, yamlLine{indent: indent, text: strings.TrimSpace(trimmed)})
	}
	return out
}

// composeBlock returns the lines nested under the single line whose text and
// indentation match header. The block ends at the next line at or above it. A
// second matching header is a duplicate key: Compose rejects it, but a
// first-match walk would silently read only one and pass an invalid file.
func composeBlock(t *testing.T, lines []yamlLine, header string, indent int) []yamlLine {
	t.Helper()
	var matches [][]yamlLine
	for i, l := range lines {
		if l.text != header || l.indent != indent {
			continue
		}
		var out []yamlLine
		for _, n := range lines[i+1:] {
			if n.indent <= indent {
				break
			}
			out = append(out, n)
		}
		matches = append(matches, out)
	}
	switch len(matches) {
	case 0:
		t.Fatalf("compose.yaml: no %q block at indent %d", header, indent)
	case 1:
		return matches[0]
	default:
		t.Fatalf("compose.yaml: %q block at indent %d appears %d times", header, indent, len(matches))
	}
	return nil
}

// composeChildKeys returns the mapping keys declared directly under a block at
// the given indentation.
func composeChildKeys(block []yamlLine, indent int) []string {
	var out []string
	for _, l := range block {
		if l.indent == indent && strings.HasSuffix(l.text, ":") {
			out = append(out, l.text)
		}
	}
	return out
}

func composeEnv(t *testing.T, block []yamlLine) map[string]string {
	t.Helper()
	envs := map[string]string{}
	for _, l := range block {
		key, value, ok := strings.Cut(l.text, ":")
		if !ok {
			t.Fatalf("compose environment entry %q is not KEY: value", l.text)
		}
		envs[strings.TrimSpace(key)] = composeUnquote(strings.TrimSpace(value))
	}
	return envs
}

func composeList(t *testing.T, block []yamlLine) []string {
	t.Helper()
	var out []string
	for _, l := range block {
		if !strings.HasPrefix(l.text, "- ") {
			t.Fatalf("compose list entry %q is not a sequence item", l.text)
		}
		out = append(out, composeUnquote(strings.TrimSpace(strings.TrimPrefix(l.text, "- "))))
	}
	return out
}

func composeScalar(t *testing.T, block []yamlLine, key string, indent int) string {
	t.Helper()
	for _, l := range block {
		if l.indent == indent && strings.HasPrefix(l.text, key) {
			return strings.TrimSpace(strings.TrimPrefix(l.text, key))
		}
	}
	t.Fatalf("compose.yaml: missing %q at indent %d", key, indent)
	return ""
}

// composeHasKey reports whether a block declares a mapping key at the given
// indentation. It matches the key prefix so an inline value (env_file: .env) is
// found as well as a nested block (secrets:).
func composeHasKey(block []yamlLine, key string, indent int) bool {
	for _, l := range block {
		if l.indent == indent && strings.HasPrefix(l.text, key) {
			return true
		}
	}
	return false
}

func composeUnquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}
