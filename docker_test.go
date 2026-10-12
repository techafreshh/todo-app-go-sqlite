package main

import (
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
)

// The Dockerfile re-declares the server's runtime interface — the env keys the
// process reads and the volume that must back the DB — as independent literals.
// This test binds the two sides so a rename or reorg on either fails here
// instead of routing the DB into the container layer while /healthz stays green.
func TestDockerfileBindsRuntimeContract(t *testing.T) {
	df := readDockerfile(t)

	envs := dockerfileEnv(t, df)
	for _, key := range []string{envPort, envDBPath} {
		if _, ok := envs[key]; !ok {
			t.Errorf("Dockerfile ENV missing %s, which main.go reads", key)
		}
	}
	for key := range envs {
		if key != envPort && key != envDBPath {
			t.Errorf("Dockerfile sets ENV %s, which the server never reads", key)
		}
	}

	// The DB path must sit under a declared volume, or writes land in the image
	// layer and are lost on restart (PRD §14.5).
	dbDir := path.Dir(envs[envDBPath])
	if volumes := dockerfileVolumes(df); !volumes[dbDir] {
		t.Errorf("DB_PATH %s is not under a declared VOLUME %v", envs[envDBPath], volumes)
	}
}

func readDockerfile(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	return string(b)
}

// dockerfileEnv maps each KEY=value on an ENV instruction to its value.
func dockerfileEnv(t *testing.T, df string) map[string]string {
	t.Helper()
	envs := map[string]string{}
	re := regexp.MustCompile(`^ENV\s+(.*)$`)
	for _, line := range dockerfileLogicalLines(df) {
		m := re.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		for _, kv := range strings.Fields(m[1]) {
			key, value, ok := strings.Cut(kv, "=")
			if !ok {
				t.Fatalf("Dockerfile ENV token %q is not KEY=value", kv)
			}
			envs[key] = value
		}
	}
	return envs
}

func dockerfileVolumes(df string) map[string]bool {
	volumes := map[string]bool{}
	re := regexp.MustCompile(`(?m)^\s*VOLUME\s+(.*)$`)
	for _, m := range re.FindAllStringSubmatch(df, -1) {
		for _, v := range strings.Fields(m[1]) {
			volumes[strings.Trim(v, `"[]`)] = true
		}
	}
	return volumes
}

// dockerfileLogicalLines joins backslash continuations so an instruction split
// across lines is read whole.
func dockerfileLogicalLines(df string) []string {
	var lines []string
	var cur strings.Builder
	for _, line := range strings.Split(df, "\n") {
		trimmed := strings.TrimRight(line, " \t")
		if strings.HasSuffix(trimmed, "\\") {
			cur.WriteString(strings.TrimSuffix(trimmed, "\\"))
			cur.WriteByte(' ')
			continue
		}
		cur.WriteString(line)
		lines = append(lines, cur.String())
		cur.Reset()
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return lines
}
