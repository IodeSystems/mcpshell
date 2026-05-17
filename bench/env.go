// Package bench runs the mcpshell LLM benchmark suite against an
// OpenAI-compatible chat-completions API.
package bench

import (
	"bufio"
	"os"
	"strings"
)

// LoadEnvFile reads KEY=VALUE lines from path into the process environment.
// Blank lines and '#' comments are skipped. A variable already present in the
// real environment is left untouched — the environment wins over the file.
// A missing file is not an error.
func LoadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return sc.Err()
}
