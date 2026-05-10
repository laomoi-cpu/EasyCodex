package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"easycodex-agent/internal/config"
)

type fakeWezTerm struct {
	lastClass   string
	lastPaneID  string
	lastLines   int
	lastEscapes bool
	lastText    string
	lastNoPaste bool
	launched    bool
}

func (fake *fakeWezTerm) Launch(ctx context.Context, class string) error {
	fake.lastClass = class
	fake.launched = true
	return nil
}

func (fake *fakeWezTerm) List(ctx context.Context, class string) (json.RawMessage, error) {
	fake.lastClass = class
	return json.RawMessage(`[{"window_id":1,"tabs":[{"tab_id":2,"panes":[{"pane_id":3}]}]}]`), nil
}

func (fake *fakeWezTerm) GetText(ctx context.Context, class, paneID string, lines int, escapes bool) (string, error) {
	fake.lastClass = class
	fake.lastPaneID = paneID
	fake.lastLines = lines
	fake.lastEscapes = escapes
	return "hello", nil
}

func (fake *fakeWezTerm) SendText(ctx context.Context, class, paneID, text string, noPaste bool) error {
	fake.lastClass = class
	fake.lastPaneID = paneID
	fake.lastText = text
	fake.lastNoPaste = noPaste
	return nil
}

func (fake *fakeWezTerm) Spawn(ctx context.Context, class, cwd string, newWindow bool, command []string) (string, error) {
	fake.lastClass = class
	return "9", nil
}

func testServer(t *testing.T) (*Server, *fakeWezTerm) {
	t.Helper()
	cfg := config.Config{
		Listen: "127.0.0.1:0",
		Root:   `D:\EasyCodex`,
		Token:  "secret",
		Instances: []config.Instance{
			{ID: "main", Name: "main", Class: "easycodex"},
		},
	}
	fake := &fakeWezTerm{}
	srv, err := New(cfg, fake, nil)
	if err != nil {
		t.Fatal(err)
	}
	return srv, fake
}

func TestHealthDoesNotRequireAuth(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestInstancesRequiresAuth(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestSessionsReturnsWezTermPayload(t *testing.T) {
	srv, fake := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/instances/main/sessions", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if fake.lastClass != "easycodex" {
		t.Fatalf("class = %q", fake.lastClass)
	}
}

func TestLaunchInstance(t *testing.T) {
	srv, fake := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/instances/main/launch", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !fake.launched || fake.lastClass != "easycodex" {
		t.Fatalf("unexpected launch: %#v", fake)
	}
}

func TestPaneTextValidatesLinesAndPaneID(t *testing.T) {
	srv, fake := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/instances/main/panes/3/text?lines=25&escapes=1", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if fake.lastPaneID != "3" || fake.lastLines != 25 || !fake.lastEscapes {
		t.Fatalf("unexpected call: %#v", fake)
	}
}

func TestSendText(t *testing.T) {
	srv, fake := testServer(t)
	body := bytes.NewBufferString(`{"text":"dir\r","noPaste":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/instances/main/panes/3/send", body)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if fake.lastText != "dir\r" || !fake.lastNoPaste {
		t.Fatalf("unexpected send: %#v", fake)
	}
}
