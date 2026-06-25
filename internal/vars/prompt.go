package vars

import "strings"

// PromptVars scans texts for {{?Name}} prompt-variable references (#86) and
// returns the distinct prompt keys — each the captured template name including
// its leading '?' marker — in first-seen order. A prompt variable signals that
// its value should be collected from the user at Send time rather than resolved
// from a static scope; injecting a resolver scope entry under the returned key
// resolves the corresponding {{?...}} reference. Use PromptLabel to turn a key
// into a human-facing prompt string.
func PromptVars(texts ...string) []string {
	var keys []string
	seen := map[string]bool{}
	for _, t := range texts {
		for _, m := range varRe.FindAllStringSubmatch(t, -1) {
			name := strings.TrimSpace(m[1])
			if !strings.HasPrefix(name, "?") {
				continue
			}
			if strings.TrimSpace(name[1:]) == "" || seen[name] {
				continue
			}
			seen[name] = true
			keys = append(keys, name)
		}
	}
	return keys
}

// PromptLabel returns the human-facing label for a prompt key produced by
// PromptVars — the name with its leading '?' marker removed and trimmed.
func PromptLabel(key string) string {
	return strings.TrimSpace(strings.TrimPrefix(key, "?"))
}
