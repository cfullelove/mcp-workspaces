package workspace

import (
	"regexp"
	"strings"
)

var (
	// invalidCharsRegex matches characters that are not lowercase letters, numbers, or hyphens.
	invalidCharsRegex = regexp.MustCompile(`[^a-z0-9-]`)
	// separatorRegex matches one or more whitespace characters or hyphens.
	separatorRegex = regexp.MustCompile(`[\s-]+`)
	// multiDashRegex matches multiple hyphens.
	multiDashRegex = regexp.MustCompile(`-{2,}`)
)

const maxSlugLength = 64

// GenerateSlug creates a filesystem-safe, unique-enough slug from a given name.
// It follows the rules in the PRD:
// 1. Lowercase normalization.
// 2. Replace whitespace and invalid characters with a hyphen.
// 3. Collapse repeated hyphens.
// 4. Trim leading/trailing hyphens.
// 5. Truncate to a safe length.
func GenerateSlug(name string) string {
	// 1. Lowercase normalization
	slug := strings.ToLower(name)

	// 2. Replace whitespace and invalid characters with a hyphen
	slug = separatorRegex.ReplaceAllString(slug, "-")
	slug = invalidCharsRegex.ReplaceAllString(slug, "") // Remove any remaining invalid chars

	// 3. Collapse repeated hyphens
	slug = multiDashRegex.ReplaceAllString(slug, "-")

	// 4. Trim leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	// 5. Truncate to a safe length
	if len(slug) > maxSlugLength {
		slug = slug[:maxSlugLength]
		// Re-trim in case we cut on a hyphen
		slug = strings.Trim(slug, "-")
	}

	return slug
}