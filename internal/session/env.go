package session

import (
	"strings"

	"github.com/idct/helena/internal/model"
)

// ParseEnvVars parses a simple "key = value" environment text (one variable per
// line) into model variables. Lines beginning with '#' are treated as disabled
// variables; blank and malformed (no '=') lines are skipped. The Secret flag is
// not represented in this text form.
func ParseEnvVars(text string) []model.Variable {
	var out []model.Variable
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		enabled := true
		if strings.HasPrefix(line, "#") {
			enabled = false
			line = strings.TrimSpace(line[1:])
			if line == "" {
				continue
			}
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		out = append(out, model.Variable{
			Enabled: enabled,
			Key:     strings.TrimSpace(key),
			Value:   strings.TrimSpace(value),
		})
	}
	return out
}

// FormatEnvVars renders variables back to the "key = value" text form, prefixing
// disabled variables with "# ".
func FormatEnvVars(vs []model.Variable) string {
	var b strings.Builder
	for _, v := range vs {
		if !v.Enabled {
			b.WriteString("# ")
		}
		b.WriteString(v.Key)
		b.WriteString(" = ")
		b.WriteString(v.Value)
		b.WriteString("\n")
	}
	return b.String()
}
