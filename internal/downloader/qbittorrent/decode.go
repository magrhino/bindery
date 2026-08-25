package qbittorrent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// snippetLimit caps how much of an unrecognised body is quoted back.
const snippetLimit = 80

// decodeJSON unmarshals a WebUI response into out, replacing the parser's own
// error with a description of what actually arrived when the body was never
// JSON to begin with.
//
// Same treatment #2128 gave Hardcover's error pages and #2105 gave NZB
// fetches: a body the parser cannot even start on is a routing problem, not a
// schema problem, and "invalid character '<' looking for beginning of value"
// names neither. The case that prompted this (#2203) was a Host field holding
// "1.2.3.4:8080/#/", whose fragment swallowed the API path so every call came
// back with the WebUI's own HTML index page.
//
// A body that parsed as JSON but did not fit the expected shape keeps the
// parser's message, which is the useful one there.
func decodeJSON(what string, data []byte, out any) error {
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode %s: %s", what, describeDecodeFailure(data, err))
	}
	return nil
}

// describeDecodeFailure explains a failed decode in terms of the body rather
// than the byte offset the parser stopped at.
func describeDecodeFailure(data []byte, err error) string {
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		// Valid JSON, wrong shape. The parser's message names the field.
		return err.Error()
	}
	trimmed := bytes.TrimSpace(data)
	switch {
	case len(trimmed) == 0:
		return "qBittorrent returned an empty response body"
	case isMarkup(trimmed):
		return markupHint()
	default:
		return fmt.Sprintf("qBittorrent returned a non-JSON response starting %s", snippet(trimmed))
	}
}

// markupHint is the message for a body that is a web page. It names the two
// settings that route a request away from the API, because those are what the
// operator can change.
func markupHint() string {
	return "qBittorrent returned an HTML page instead of JSON, which is what its WebUI serves for any URL outside the API. " +
		"Check that the download client's Host is a bare hostname or IP with no port and no path in it, " +
		"and that URL Base matches the path your reverse proxy serves qBittorrent on"
}

// isMarkup reports whether the body opens an HTML or XML document. The
// leading angle bracket is the whole test: it is the byte the JSON parser
// tripped on, and no JSON document starts with one.
func isMarkup(trimmed []byte) bool {
	return trimmed[0] == '<'
}

// snippet renders the head of an unrecognised body as a single quoted line,
// with runs of whitespace and control characters collapsed so a log line
// stays one line.
func snippet(trimmed []byte) string {
	var b strings.Builder
	space := false
	for _, r := range string(trimmed) {
		if b.Len() >= snippetLimit {
			break
		}
		if unicode.IsSpace(r) || !unicode.IsPrint(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	return fmt.Sprintf("%q", b.String())
}

// checkVersionBody reports whether the /api/v2/app/version response really
// came from the API. It is the one probe the Test button makes, and the
// endpoint answers in plain text, so nothing downstream would otherwise
// notice that an HTML page came back with HTTP 200 instead — which is exactly
// how the misconfigured Host in #2203 tested green and then failed on every
// poll.
func checkVersionBody(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	switch {
	case len(trimmed) == 0:
		return errors.New("it returned an empty response for the API version rather than a version string")
	case isMarkup(trimmed):
		return errors.New(markupHint())
	default:
		return nil
	}
}
