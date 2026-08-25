package api

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/vavallee/bindery/internal/httpsec"
	"github.com/vavallee/bindery/internal/models"
)

// qbitStub is a minimal qBittorrent that records the password it was asked to
// log in with, so the tests below can assert what actually left the process.
func qbitStub(t *testing.T) (host string, port int, seen *string) {
	t.Helper()
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			got = r.PostFormValue("password")
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("5.1.4"))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte("[]"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	hostStr, portStr, _ := net.SplitHostPort(u.Host)
	p, _ := strconv.Atoi(portStr)
	return hostStr, p, &got
}

// TestDownloadClientTestConfig_HydratesStoredCredential covers the edit form's
// Test button after #2213: the browser no longer holds the stored password, so
// a blank credential has to be filled in from the saved row.
func TestDownloadClientTestConfig_HydratesStoredCredential(t *testing.T) {
	defer httpsec.AllowLoopbackForTests()()
	host, port, seen := qbitStub(t)

	h, clients := downloadClientFixture(t)
	seeded := seedDownloadClient(t, clients, models.DownloadClient{
		Name: "qBit", Type: "qbittorrent", Host: host, Port: port, Username: "admin", Password: "stored-pass", Category: "books", Enabled: true,
	})

	body := `{"id":` + strconv.FormatInt(seeded.ID, 10) + `,"name":"qBit","type":"qbittorrent","host":"` + host +
		`","port":` + strconv.Itoa(port) + `,"username":"admin","category":"ebooks"}`
	rec := httptest.NewRecorder()
	h.TestConfig(rec, httptest.NewRequest(http.MethodPost, "/downloadclient/test", bytes.NewBufferString(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if *seen != "stored-pass" {
		t.Errorf("probe used password %q, want the stored one", *seen)
	}
}

// TestDownloadClientTestConfig_RefusesHydrationForAnotherTarget is the reason
// the hydration is fenced: without the guard, the endpoint would be a way to
// aim a stored credential at a host the caller picked.
func TestDownloadClientTestConfig_RefusesHydrationForAnotherTarget(t *testing.T) {
	defer httpsec.AllowLoopbackForTests()()
	savedHost, savedPort, _ := qbitStub(t)
	otherHost, otherPort, otherSeen := qbitStub(t)

	h, clients := downloadClientFixture(t)
	seeded := seedDownloadClient(t, clients, models.DownloadClient{
		Name: "qBit", Type: "qbittorrent", Host: savedHost, Port: savedPort, Username: "admin", Password: "stored-pass", Category: "books", Enabled: true,
	})

	body := `{"id":` + strconv.FormatInt(seeded.ID, 10) + `,"name":"qBit","type":"qbittorrent","host":"` + otherHost +
		`","port":` + strconv.Itoa(otherPort) + `,"username":"admin"}`
	rec := httptest.NewRecorder()
	h.TestConfig(rec, httptest.NewRequest(http.MethodPost, "/downloadclient/test", bytes.NewBufferString(body)))
	if *otherSeen != "" {
		t.Errorf("stored password leaked to a redirected probe: %q", *otherSeen)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("stored-pass")) {
		t.Errorf("test-config response leaked the stored password: %s", rec.Body.String())
	}
}

// TestDownloadClientTest_ResponseHasNoCredential guards the test-by-id shape.
func TestDownloadClientTest_ResponseHasNoCredential(t *testing.T) {
	defer httpsec.AllowLoopbackForTests()()
	host, port, _ := qbitStub(t)

	h, clients := downloadClientFixture(t)
	seeded := seedDownloadClient(t, clients, models.DownloadClient{
		Name: "qBit", Type: "qbittorrent", Host: host, Port: port, Username: "admin", Password: "stored-pass", Category: "books", Enabled: true,
	})
	idStr := strconv.FormatInt(seeded.ID, 10)
	rec := httptest.NewRecorder()
	h.Test(rec, withURLParam(httptest.NewRequest(http.MethodPost, "/downloadclient/"+idStr+"/test", nil), "id", idStr))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("stored-pass")) {
		t.Errorf("test response leaked the stored password: %s", rec.Body.String())
	}
}

// TestDownloadClientTestConfig_HydrationEdges keeps the fenced lookup honest at
// its boundaries: an id that matches nothing must not stop the probe, and a
// body that already carries both credentials must not trigger a lookup at all.
func TestDownloadClientTestConfig_HydrationEdges(t *testing.T) {
	defer httpsec.AllowLoopbackForTests()()

	t.Run("unknown id still probes with the supplied credential", func(t *testing.T) {
		host, port, seen := qbitStub(t)
		h, _ := downloadClientFixture(t)
		body := `{"id":999,"name":"qBit","type":"qbittorrent","host":"` + host +
			`","port":` + strconv.Itoa(port) + `,"username":"admin","password":"body-pass"}`
		rec := httptest.NewRecorder()
		h.TestConfig(rec, httptest.NewRequest(http.MethodPost, "/downloadclient/test", bytes.NewBufferString(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if *seen != "body-pass" {
			t.Errorf("probe used password %q, want body-pass", *seen)
		}
	})

	t.Run("a fully supplied body wins over the stored row", func(t *testing.T) {
		host, port, seen := qbitStub(t)
		h, clients := downloadClientFixture(t)
		seeded := seedDownloadClient(t, clients, models.DownloadClient{
			Name: "qBit", Type: "qbittorrent", Host: host, Port: port, Username: "admin", Password: "stored-pass", Category: "books", Enabled: true,
		})
		body := `{"id":` + strconv.FormatInt(seeded.ID, 10) + `,"name":"qBit","type":"qbittorrent","host":"` + host +
			`","port":` + strconv.Itoa(port) + `,"username":"admin","apiKey":"body-key","password":"body-pass"}`
		rec := httptest.NewRecorder()
		h.TestConfig(rec, httptest.NewRequest(http.MethodPost, "/downloadclient/test", bytes.NewBufferString(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if *seen != "body-pass" {
			t.Errorf("probe used password %q, want body-pass", *seen)
		}
	})
}
