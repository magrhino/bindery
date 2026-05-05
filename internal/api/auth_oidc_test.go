package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vavallee/bindery/internal/auth/oidc"
)

// TestGetRedirectBase verifies the endpoint returns the resolved base URL
// from the injected resolver plus the callback path template, ready for the
// admin UI to render a live preview.
func TestGetRedirectBase(t *testing.T) {
	mgr := oidc.NewManager()
	h := NewOIDCHandler(mgr, nil, nil, nil, func(_ *http.Request) string {
		return "https://bindery.example.com"
	})

	rec := httptest.NewRecorder()
	h.GetRedirectBase(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/redirect-base", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Base         string `json:"base"`
		CallbackPath string `json:"callback_path"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse body: %v (body=%s)", err, rec.Body.String())
	}
	if got.Base != "https://bindery.example.com" {
		t.Fatalf("base=%q, want https://bindery.example.com", got.Base)
	}
	if got.CallbackPath != "/api/v1/auth/oidc/{id}/callback" {
		t.Fatalf("callback_path=%q, want /api/v1/auth/oidc/{id}/callback", got.CallbackPath)
	}
}

// TestGetRedirectBase_ResolverHonorsRequest verifies the resolver receives
// the actual request — important because real deploys derive the base URL
// from forwarded headers on the incoming request.
func TestGetRedirectBase_ResolverHonorsRequest(t *testing.T) {
	mgr := oidc.NewManager()
	h := NewOIDCHandler(mgr, nil, nil, nil, func(r *http.Request) string {
		return r.Header.Get("X-Forwarded-Proto") + "://" + r.Header.Get("X-Forwarded-Host")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/redirect-base", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "bindery.public.example")
	rec := httptest.NewRecorder()
	h.GetRedirectBase(rec, req)

	var got struct {
		Base string `json:"base"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Base != "https://bindery.public.example" {
		t.Fatalf("resolver should see request headers, got base=%q", got.Base)
	}
}
