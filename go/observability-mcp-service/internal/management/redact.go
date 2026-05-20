package management

import "regexp"

var redactPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(token|secret|password|authorization)=\S+`),
	regexp.MustCompile(`(?i)bearer\s+\S+`),
}

func boundOutput(value string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value, false
	}
	return value[:maxBytes], true
}

func redactOutput(value string) (string, int) {
	count := 0
	for _, pattern := range redactPatterns {
		value = pattern.ReplaceAllStringFunc(value, func(match string) string {
			count++
			if parts := pattern.FindStringSubmatch(match); len(parts) > 1 {
				return parts[1] + "=[REDACTED]"
			}
			return "[REDACTED]"
		})
	}
	return value, count
}
