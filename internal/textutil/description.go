package textutil

import (
	"html"
	"regexp"
	"strings"
)

var (
	breakTagRE = regexp.MustCompile(`(?i)<\s*br\s*/?\s*>`)
	blockTagRE = regexp.MustCompile(`(?i)</?\s*(p|div|li|ul|ol|blockquote|h[1-6])(?:\s+[^>]*)?>`)
	tagRE      = regexp.MustCompile(`<[^>]+>`)
)

// CleanDescription normalizes provider descriptions for plain-text UI display.
func CleanDescription(description string) string {
	description = strings.TrimSpace(description)
	if description == "" {
		return ""
	}

	description = html.UnescapeString(description)
	description = breakTagRE.ReplaceAllString(description, "\n")
	description = blockTagRE.ReplaceAllString(description, "\n\n")
	description = tagRE.ReplaceAllString(description, "")
	description = html.UnescapeString(description)
	description = strings.ReplaceAll(description, "\r\n", "\n")
	description = strings.ReplaceAll(description, "\r", "\n")

	parts := strings.Split(description, "\n")
	paragraphs := make([]string, 0, len(parts))
	var current []string
	flush := func() {
		if len(current) == 0 {
			return
		}
		paragraph := strings.Join(current, " ")
		paragraph = strings.Join(strings.Fields(paragraph), " ")
		if paragraph != "" {
			paragraphs = append(paragraphs, paragraph)
		}
		current = nil
	}
	for _, part := range parts {
		part = strings.Join(strings.Fields(part), " ")
		if part == "" {
			flush()
			continue
		}
		current = append(current, part)
	}
	flush()
	return strings.Join(paragraphs, "\n\n")
}
