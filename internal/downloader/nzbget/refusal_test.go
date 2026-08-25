package nzbget

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestEditQueueRefusalIsAnError covers NZBGet's half of #2192. NZBGet declines
// an editqueue command with HTTP 200, no JSON-RPC error object and a result of
// false, so call(), which catches transport failures, non-200s and RPC
// faults, returns nil. Remove and RemoveHistory decoded that false into an
// editQueueResponse and never read it, reporting the refusal as a removal.
func TestEditQueueRefusalIsAnError(t *testing.T) {
	cases := []struct {
		name        string
		call        func(*Client) error
		wantCommand string
	}{
		{"remove (keep files)", func(c *Client) error { return c.Remove(context.Background(), 101, false) }, "GroupParkDelete"},
		{"remove (delete files)", func(c *Client) error { return c.Remove(context.Background(), 101, true) }, "GroupDelete"},
		{"remove history", func(c *Client) error { return c.RemoveHistory(context.Background(), 101) }, "HistoryDelete"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// The refusal shape: 200 OK, no "error" member, result false.
				_ = json.NewEncoder(w).Encode(editQueueResponse{Result: false})
			}))
			defer srv.Close()

			host, port := serverHostPort(t, srv.URL)
			c := New(host, port, "", "", "", false)

			err := tc.call(c)
			if err == nil {
				t.Fatal("expected an error when NZBGet returns result false, got nil")
			}
			if !strings.Contains(err.Error(), "NZBGet") {
				t.Errorf("error should name the client, got %q", err)
			}
			if !strings.Contains(err.Error(), tc.wantCommand) {
				t.Errorf("error should name the refused command %q, got %q", tc.wantCommand, err)
			}
			if !strings.Contains(err.Error(), "101") {
				t.Errorf("error should name the nzb id, got %q", err)
			}
		})
	}
}

// TestEditQueueSuccessIsNil pins the other half: an accepted command still
// returns nil, so the new check can't turn a working removal into a warning.
func TestEditQueueSuccessIsNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(editQueueResponse{Result: true})
	}))
	defer srv.Close()

	host, port := serverHostPort(t, srv.URL)
	c := New(host, port, "", "", "", false)

	if err := c.Remove(context.Background(), 101, false); err != nil {
		t.Errorf("Remove: %v", err)
	}
	if err := c.RemoveHistory(context.Background(), 101); err != nil {
		t.Errorf("RemoveHistory: %v", err)
	}
}
