package session

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// dotEnvFile is the conventional name of the collection-root environment file
// (#84) — a Bruno/Postman-compatible KEY=VALUE list that seeds the lowest
// static resolver scope.
const dotEnvFile = ".env"

// activeDotEnvVars returns the active collection's .env variables (#84), the
// lowest-precedence static scope — a collection / environment / request value
// of the same name overrides it. The parsed map is cached per collection dir;
// reload() drops the cache so a reopened collection re-reads from disk. Missing
// or unreadable files yield an empty map. Call from the UI goroutine.
func (s *Session) activeDotEnvVars() map[string]string {
	dir := s.ActiveCollectionDir()
	if dir == "" {
		return map[string]string{}
	}
	if s.dotEnv == nil {
		s.dotEnv = map[string]map[string]string{}
	}
	if cached, ok := s.dotEnv[dir]; ok {
		return cached
	}
	data, err := os.ReadFile(filepath.Join(dir, dotEnvFile))
	if err != nil {
		s.dotEnv[dir] = map[string]string{}
		return s.dotEnv[dir]
	}
	parsed := parseDotEnv(data)
	s.dotEnv[dir] = parsed
	return parsed
}

// SnapshotActiveDotEnvVars returns a copy of the active collection's .env
// variables for the Send worker (same rationale as SnapshotActiveCollectionVars
// — the worker must not read session state mid-Send). The map is owned by the
// caller.
func (s *Session) SnapshotActiveDotEnvVars() map[string]string {
	src := s.activeDotEnvVars()
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// parseDotEnv parses a .env document into a name->value map. It accepts
// `KEY=VALUE` lines, ignores blank lines and `#` comments, tolerates an
// optional leading `export `, trims surrounding whitespace from the key, and
// strips a single matching pair of surrounding single or double quotes from the
// value (leaving `=` characters inside the value intact). Malformed lines (no
// `=`) are skipped.
func parseDotEnv(data []byte) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		if key == "" {
			continue
		}
		out[key] = unquoteDotEnv(strings.TrimSpace(line[eq+1:]))
	}
	return out
}

// unquoteDotEnv strips a single matching pair of surrounding single or double
// quotes from a .env value; an unquoted value is returned unchanged.
func unquoteDotEnv(v string) string {
	if len(v) >= 2 {
		if c := v[0]; (c == '"' || c == '\'') && v[len(v)-1] == c {
			return v[1 : len(v)-1]
		}
	}
	return v
}
