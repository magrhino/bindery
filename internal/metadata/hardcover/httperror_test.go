package hardcover

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// errorResponse builds a non-200 HTTP response with a verbatim body.
func errorResponse(status int, contentType, body string) *http.Response {
	header := make(http.Header)
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     header,
	}
}

// hardcoverHTMLErrorPage is the page Hardcover served during the 2026-08 outage
// reported in #2128. The old client pasted the first 512 bytes of it into the
// error, so the Settings UI rendered markup at the operator instead of saying
// which side had failed.
const hardcoverHTMLErrorPage = `<!doctype html>

<html lang="en">

<head>
  <title>Internal Server Error | Hardcover</title>
  <meta name="viewport" content="width=device-width, initial-scale=1">
</head>

<body>
  <h1>Something went wrong</h1>
  <p>Our team has been notified.</p>
</body>

</html>`

// TestQueryHTTPErrorClassification pins how a non-200 Hardcover response is
// reported. The rule the table encodes: a rejected token says so, a structured
// JSON error is quoted compactly, and anything else is named as an upstream
// failure without echoing the body (#2128).
func TestQueryHTTPErrorClassification(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
		want        []string
		notWant     []string
	}{
		{
			name:        "401 with the auth error envelope",
			status:      http.StatusUnauthorized,
			contentType: "application/json",
			body:        `{"error":"invalid_token","error_description":"Invalid or expired token"}`,
			want:        []string{"token rejected", "HTTP 401", "Invalid or expired token"},
		},
		{
			name:        "401 for a JWT-shaped token",
			status:      http.StatusUnauthorized,
			contentType: "application/json",
			body:        `{"error":"invalid_token","error_description":"Invalid JWT"}`,
			want:        []string{"token rejected", "HTTP 401", "Invalid JWT"},
		},
		{
			name:   "401 with no body at all",
			status: http.StatusUnauthorized,
			body:   "",
			want:   []string{"token rejected", "HTTP 401", "Hardcover API token"},
		},
		{
			name:        "400 for a malformed Authorization header",
			status:      http.StatusBadRequest,
			contentType: "application/json",
			body:        `{"error":"invalid_request","error_description":"Invalid Authorization format. Use 'Bearer <token>'"}`,
			want:        []string{"HTTP 400", "Invalid Authorization format"},
			// Angle brackets never survive into an error message, wherever
			// upstream put them.
			notWant: []string{"<", ">"},
		},
		{
			name:        "403 for a refused query operator is not a token problem",
			status:      http.StatusForbidden,
			contentType: "application/json",
			body:        `{"error":"ilike and related operations are not permitted on this server."}`,
			want:        []string{"HTTP 403", "ilike and related operations are not permitted"},
			notWant:     []string{"token rejected"},
		},
		{
			name:        "403 carrying an auth error is a token problem",
			status:      http.StatusForbidden,
			contentType: "application/json",
			body:        `{"error":"insufficient_scope","error_description":"Token lacks the required scope"}`,
			want:        []string{"token rejected", "HTTP 403", "Token lacks the required scope"},
		},
		{
			name:        "403 naming a token but rejecting nothing is not a token problem",
			status:      http.StatusForbidden,
			contentType: "application/json",
			body:        `{"error":"rate_limited","error_description":"Token bucket exhausted, retry after 30s"}`,
			want:        []string{"HTTP 403", "Token bucket exhausted"},
			notWant:     []string{"token rejected"},
		},
		{
			name:        "403 for an unauthenticated request is a token problem",
			status:      http.StatusForbidden,
			contentType: "application/json",
			body:        `{"error":"Unable to verify token"}`,
			want:        []string{"token rejected", "HTTP 403", "Unable to verify token"},
		},
		{
			name:        "500 with an HTML error page",
			status:      http.StatusInternalServerError,
			contentType: "text/html; charset=utf-8",
			body:        hardcoverHTMLErrorPage,
			want:        []string{"HTTP 500", "non-JSON", "Hardcover-side failure"},
			// The whole point of #2128: not one byte of the page is quoted.
			notWant: []string{"<", ">", "doctype", "Internal Server Error", "Something went wrong", "viewport"},
		},
		{
			name:   "500 with an empty body",
			status: http.StatusInternalServerError,
			body:   "",
			want:   []string{"HTTP 500", "empty response body", "Hardcover-side failure"},
		},
		{
			name:        "502 with a plain-text body",
			status:      http.StatusBadGateway,
			contentType: "text/plain",
			body:        "Bad Gateway",
			want:        []string{"HTTP 502", "non-JSON", "Hardcover-side failure"},
			notWant:     []string{"Bad Gateway"},
		},
		{
			name:        "503 with JSON that carries no description",
			status:      http.StatusServiceUnavailable,
			contentType: "application/json",
			body:        `{"retry_after":30}`,
			want:        []string{"HTTP 503", "no description", "Hardcover-side failure"},
		},
		{
			name:        "504 with a truncated HTML page",
			status:      http.StatusGatewayTimeout,
			contentType: "text/html",
			body:        strings.Repeat("<div>gateway timeout</div>", 4096),
			want:        []string{"HTTP 504", "non-JSON"},
			notWant:     []string{"<", "gateway timeout"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newMockClient(func(*http.Request) (*http.Response, error) {
				return errorResponse(tt.status, tt.contentType, tt.body), nil
			})

			var out struct{}
			err := c.query(context.Background(), "query Test { __typename }", nil, &out)
			if err == nil {
				t.Fatalf("query: want an error for HTTP %d, got nil", tt.status)
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(err.Error(), notWant) {
					t.Errorf("error %q must not contain %q", err, notWant)
				}
			}
		})
	}
}

// TestQueryHTTPErrorDetailIsBounded keeps a chatty upstream from pasting an
// unbounded blob into the Settings UI, even when it arrives as valid JSON.
func TestQueryHTTPErrorDetailIsBounded(t *testing.T) {
	c := newMockClient(func(*http.Request) (*http.Response, error) {
		return errorResponse(http.StatusInternalServerError, "application/json",
			`{"error":"boom","error_description":"`+strings.Repeat("y", 4000)+`"}`), nil
	})

	var out struct{}
	err := c.query(context.Background(), "query Test { __typename }", nil, &out)
	if err == nil {
		t.Fatal("query: want an error, got nil")
	}
	if len(err.Error()) > 300 {
		t.Fatalf("error message is %d bytes, want it capped: %q", len(err.Error()), err)
	}
	if !strings.Contains(err.Error(), "…") {
		t.Fatalf("truncated detail should be marked as truncated: %q", err)
	}
}

// TestQueryHTTPErrorRedactsSecrets guards the redaction that was already applied
// to the raw body before #2128 and must still apply to the text we quote.
func TestQueryHTTPErrorRedactsSecrets(t *testing.T) {
	c := newMockClient(func(*http.Request) (*http.Response, error) {
		return errorResponse(http.StatusForbidden, "application/json",
			`{"error":"denied","error_description":"blocked callback https://hardcover.app/cb?apikey=sup3rsecret"}`), nil
	})

	var out struct{}
	err := c.query(context.Background(), "query Test { __typename }", nil, &out)
	if err == nil {
		t.Fatal("query: want an error, got nil")
	}
	if strings.Contains(err.Error(), "sup3rsecret") {
		t.Fatalf("error leaks a secret: %q", err)
	}
	if !strings.Contains(err.Error(), "REDACTED") {
		t.Fatalf("error should show the redaction marker: %q", err)
	}
}

// TestQueryHTTPErrorBodyReadIsBounded pins the read cap on error bodies: an
// upstream error page can be arbitrarily large and none of it is used.
func TestQueryHTTPErrorBodyReadIsBounded(t *testing.T) {
	body := &countingBodyReader{remaining: hardcoverErrorResponseBodyLimit * 4}
	c := newMockClient(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(body),
			Header:     make(http.Header),
		}, nil
	})

	var out struct{}
	if err := c.query(context.Background(), "query Test { __typename }", nil, &out); err == nil {
		t.Fatal("query: want an error, got nil")
	}
	if body.read != hardcoverErrorResponseBodyLimit {
		t.Fatalf("read bytes = %d, want %d", body.read, hardcoverErrorResponseBodyLimit)
	}
}

// TestQueryGraphQLErrorEnvelopeUnchanged is a regression guard: a 200 carrying
// GraphQL errors is a different failure from a non-200 and keeps reporting the
// upstream messages verbatim.
func TestQueryGraphQLErrorEnvelopeUnchanged(t *testing.T) {
	c := newMockClient(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"errors":[{"message":"field 'language' not found in type: 'books'","extensions":{"code":"validation-failed"}}]}`)),
			Header: make(http.Header),
		}, nil
	})

	var out struct{}
	err := c.query(context.Background(), "query Test { __typename }", nil, &out)
	if err == nil {
		t.Fatal("query: want an error for a GraphQL error envelope, got nil")
	}
	for _, want := range []string{"GraphQL:", "field 'language' not found in type: 'books'", "validation-failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

// TestQuerySuccessStillDecodes proves the classification did not disturb the
// happy path.
func TestQuerySuccessStillDecodes(t *testing.T) {
	c := newMockClient(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"books":[{"id":42,"title":"Dune"}]}}`)),
			Header:     make(http.Header),
		}, nil
	})

	var out struct {
		Data struct {
			Books []struct {
				ID    int    `json:"id"`
				Title string `json:"title"`
			} `json:"books"`
		} `json:"data"`
	}
	if err := c.query(context.Background(), "query Test { __typename }", nil, &out); err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(out.Data.Books) != 1 || out.Data.Books[0].Title != "Dune" {
		t.Fatalf("decoded %+v, want one book titled Dune", out.Data)
	}
}

// TestSearchBooksUpstreamErrorMessage is the end-to-end shape of the string the
// Settings UI shows for a Hardcover outage, caller prefix included.
func TestSearchBooksUpstreamErrorMessage(t *testing.T) {
	c := newMockClient(func(*http.Request) (*http.Response, error) {
		return errorResponse(http.StatusInternalServerError, "text/html", hardcoverHTMLErrorPage), nil
	})

	_, err := c.SearchBooks(context.Background(), "Dune")
	if err == nil {
		t.Fatal("SearchBooks: want an error, got nil")
	}
	const want = "hardcover search books: HTTP 500 (upstream returned a non-JSON response, likely an error page, " +
		"so this is a Hardcover-side failure rather than a token problem)"
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
}

// TestQuerySendsPATTokenAsBearer pins the auth scheme against the premise of
// #2128, which read the outage as Hardcover having moved to a token format the
// client could not send. hc_pat_ tokens authenticate as ordinary Bearer
// credentials (verified live against api.hardcover.app on 2026-08-24: a
// fabricated hc_pat_ value is answered 401 "Invalid or expired token", while a
// bare non-Bearer Authorization header is answered 400), so the token travels
// verbatim and no format sniffing exists to drift out of date.
func TestQuerySendsPATTokenAsBearer(t *testing.T) {
	const pat = "hc_pat_totallyfakevalue000000000000000000000000"
	var gotAuth string
	c := newMockClient(func(r *http.Request) (*http.Response, error) {
		gotAuth = r.Header.Get("Authorization")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"data":{}}`)),
			Header:     make(http.Header),
		}, nil
	}).WithToken(pat)

	var out struct{}
	if err := c.query(context.Background(), "query Test { __typename }", nil, &out); err != nil {
		t.Fatalf("query: %v", err)
	}
	if gotAuth != "Bearer "+pat {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer "+pat)
	}
}
