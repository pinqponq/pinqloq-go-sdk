package pinqloq

import "strings"

func matchesAnyPathPrefix(path string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return false
	}

	lowerPath := strings.ToLower(path)
	for _, prefix := range prefixes {
		if matchesSegmentPrefix(lowerPath, strings.ToLower(prefix)) {
			return true
		}
	}
	return false
}

func matchesSegmentPrefix(lowerPath, prefix string) bool {
	normalized := prefix
	if !strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}
	trimmed := normalized
	if len(trimmed) > 1 && strings.HasSuffix(trimmed, "/") {
		trimmed = trimmed[:len(trimmed)-1]
	}

	if !strings.HasPrefix(lowerPath, trimmed) {
		return false
	}

	return len(lowerPath) == len(trimmed) || lowerPath[len(trimmed)] == '/'
}
