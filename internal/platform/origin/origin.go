package origin

import (
	"errors"
	"slices"
	"strings"
)

var DefaultAllowedOrigins = []string{
	"http://localhost:5173",
	"http://127.0.0.1:5173",
}

var ErrOriginNotAllowed = errors.New("Origin is not allowed")

func NormalizeOrigin(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func GetAllowedOrigins(frontendURL string) []string {
	if strings.TrimSpace(frontendURL) == "" {
		return slices.Clone(DefaultAllowedOrigins)
	}

	parts := strings.Split(frontendURL, ",")
	parsed := make([]string, 0, len(parts))
	for _, part := range parts {
		norm := NormalizeOrigin(part)
		if norm != "" {
			parsed = append(parsed, norm)
		}
	}

	if len(parsed) == 0 {
		return slices.Clone(DefaultAllowedOrigins)
	}
	return parsed
}

// ResolveAllowedOrigin validates an incoming Origin header against allowedOrigins
// to prevent Open Redirect vulnerabilities.
func ResolveAllowedOrigin(originHeader string, allowedOrigins []string) (string, error) {
	if len(allowedOrigins) == 0 {
		allowedOrigins = DefaultAllowedOrigins
	}

	fallback := allowedOrigins[0]
	normalized := NormalizeOrigin(originHeader)
	if normalized == "" {
		return fallback, nil
	}

	if !slices.Contains(allowedOrigins, normalized) {
		return "", ErrOriginNotAllowed
	}

	return normalized, nil
}
