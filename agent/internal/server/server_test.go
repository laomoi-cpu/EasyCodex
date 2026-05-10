package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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
	sendCalls   []sendCall
	launched    bool
	killed      bool
}

type sendCall struct {
	class   string
	paneID  string
	text    string
	noPaste bool
}

func (fake *fakeWezTerm) Launch(ctx context.Context, class string) (int, error) {
	fake.lastClass = class
	fake.launched = true
	return 1234, nil
}

func (fake *fakeWezTerm) List(ctx context.Context, class string) (json.RawMessage, error) {
	fake.lastClass = class
	return json.RawMessage(`[
		{
			"window_id": 1,
			"window_title": "main window",
			"tab_id": 2,
			"tab_title": "work",
			"pane_id": 3,
			"title": "cmd.exe",
			"cwd": "file:///D:/mgame/",
			"workspace": "default",
			"is_active": true,
			"is_zoomed": false,
			"cursor_x": 1,
			"cursor_y": 2,
			"cursor_shape": "Default",
			"cursor_visibility": "Visible",
			"left_col": 0,
			"top_row": 0,
			"tty_name": null,
			"size": {"cols": 80, "rows": 24}
		},
		{
			"window_id": 1,
			"window_title": "main window",
			"tab_id": 2,
			"tab_title": "work",
			"pane_id": 4,
			"title": "codex",
			"cwd": "file:///D:/mgame/",
			"workspace": "default",
			"is_active": false,
			"is_zoomed": false,
			"cursor_x": 3,
			"cursor_y": 4,
			"cursor_shape": "Default",
			"cursor_visibility": "Visible",
			"left_col": 0,
			"top_row": 0,
			"tty_name": null,
			"size": {"cols": 80, "rows": 24}
		}
	]`), nil
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
	fake.sendCalls = append(fake.sendCalls, sendCall{
		class:   class,
		paneID:  paneID,
		text:    text,
		noPaste: noPaste,
	})
	return nil
}

func (fake *fakeWezTerm) KillPane(ctx context.Context, class, paneID string) error {
	fake.lastClass = class
	fake.lastPaneID = paneID
	fake.killed = true
	return nil
}

func (fake *fakeWezTerm) Spawn(ctx context.Context, class, paneID, cwd string, newWindow bool, command []string) (string, error) {
	fake.lastClass = class
	fake.lastPaneID = paneID
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

func TestPairingRequiresLocalhost(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/pairing", nil)
	req.RemoteAddr = "192.168.1.20:12345"
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestPairingReturnsConnectionInfo(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/pairing", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Service string `json:"service"`
			Token   string `json:"token"`
			Network struct {
				Listen     string `json:"listen"`
				LANEnabled bool   `json:"lanEnabled"`
			} `json:"network"`
			Instances []instanceResponse `json:"instances"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !payload.OK || payload.Data.Service != "easycodex-agent" || payload.Data.Token != "secret" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if payload.Data.Network.Listen != "127.0.0.1:0" || len(payload.Data.Instances) != 1 {
		t.Fatalf("unexpected pairing data: %#v", payload.Data)
	}
}

func TestSettingsSaveWritesConfigAndUpdatesAuth(t *testing.T) {
	cfg := config.Defaults()
	cfg.Listen = "127.0.0.1:0"
	cfg.Root = `D:\EasyCodex`
	cfg.Token = "old-secret"
	path := filepath.Join(t.TempDir(), "config.json")
	fake := &fakeWezTerm{}
	srv, err := NewWithConfigPath(cfg, path, fake, nil)
	if err != nil {
		t.Fatal(err)
	}

	body := bytes.NewBufferString(`{
		"listen":"127.0.0.1:0",
		"root":"D:\\EasyCodex",
		"token":"new-secret",
		"regenerateTokenOnStart":true,
		"publicBaseUrl":"http://100.64.1.2:8765",
		"commandTimeoutSeconds":5,
		"autoLaunch":["main"],
		"closeLaunchedGuiOnExit":false,
		"instances":[{"id":"main","name":"main","class":"easycodex"}],
		"mobileDefaults":{"instanceId":"main","cwd":"D:\\mgame","command":["cmd.exe","/k","codex"]}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/settings", body)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	loaded, found, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !found || loaded.Token != "new-secret" || !loaded.RegenerateTokenOnStart || loaded.PublicBaseURL != "http://100.64.1.2:8765" {
		t.Fatalf("unexpected saved config: found=%v cfg=%#v", found, loaded)
	}

	authReq := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	authReq.Header.Set("Authorization", "Bearer new-secret")
	authRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusOK {
		t.Fatalf("auth status = %d body = %s", authRec.Code, authRec.Body.String())
	}
}

func TestPairingPageIncludesPublicBaseURL(t *testing.T) {
	cfg := config.Defaults()
	cfg.Listen = "127.0.0.1:0"
	cfg.Root = `D:\EasyCodex`
	cfg.Token = "secret"
	cfg.PublicBaseURL = "http://100.64.1.2:8765"
	fake := &fakeWezTerm{}
	srv, err := New(cfg, fake, nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/pairing", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "http://100.64.1.2:8765") || !strings.Contains(body, "Public") {
		t.Fatalf("expected public pairing card: %s", body)
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
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Instance string          `json:"instance"`
			Windows  []windowSession `json:"windows"`
			Panes    []paneSession   `json:"panes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !payload.OK {
		t.Fatalf("expected ok response: %s", rec.Body.String())
	}
	if payload.Data.Instance != "main" || len(payload.Data.Windows) != 1 || len(payload.Data.Windows[0].Tabs) != 1 {
		t.Fatalf("unexpected session tree: %#v", payload.Data)
	}
	if len(payload.Data.Panes) != 2 || payload.Data.Panes[0].PaneID != "3" || payload.Data.Panes[1].PaneID != "4" {
		t.Fatalf("unexpected panes: %#v", payload.Data.Panes)
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

func TestPaneSnapshotReturnsHashAndTextWhenChanged(t *testing.T) {
	srv, fake := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/instances/main/panes/3/snapshot?lines=25&escapes=1", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if fake.lastPaneID != "3" || fake.lastLines != 25 || !fake.lastEscapes {
		t.Fatalf("unexpected call: %#v", fake)
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			PaneID    string `json:"paneId"`
			Text      string `json:"text"`
			Hash      string `json:"hash"`
			Changed   bool   `json:"changed"`
			LineCount int    `json:"lineCount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !payload.OK || payload.Data.PaneID != "3" || payload.Data.Text != "hello" || !payload.Data.Changed {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if payload.Data.Hash != hashText("hello") {
		t.Fatalf("hash = %q", payload.Data.Hash)
	}
	if payload.Data.LineCount != 1 {
		t.Fatalf("line count = %d", payload.Data.LineCount)
	}
}

func TestPaneSnapshotOmitsTextWhenUnchanged(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/instances/main/panes/3/snapshot?since="+hashText("hello"), nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Text    string `json:"text"`
			Hash    string `json:"hash"`
			Changed bool   `json:"changed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !payload.OK || payload.Data.Changed || payload.Data.Text != "" || payload.Data.Hash != hashText("hello") {
		t.Fatalf("unexpected payload: %#v", payload)
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

func TestSendTextWithEnterSendsReturnSeparately(t *testing.T) {
	srv, fake := testServer(t)
	body := bytes.NewBufferString(`{"text":"codex\r","noPaste":true,"enter":true,"enterDelayMillis":-1}`)
	req := httptest.NewRequest(http.MethodPost, "/api/instances/main/panes/3/send", body)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if len(fake.sendCalls) != 2 {
		t.Fatalf("send calls = %#v", fake.sendCalls)
	}
	if fake.sendCalls[0].text != "codex" || !fake.sendCalls[0].noPaste {
		t.Fatalf("unexpected text call: %#v", fake.sendCalls[0])
	}
	if fake.sendCalls[1].text != "\r" || !fake.sendCalls[1].noPaste {
		t.Fatalf("unexpected enter call: %#v", fake.sendCalls[1])
	}
}

func TestSendTextBase64PreservesChinese(t *testing.T) {
	srv, fake := testServer(t)
	body := bytes.NewBufferString(`{"textBase64":"5YiG5p6Q6aG555uu55uu5b2V","enter":true,"enterDelayMillis":-1}`)
	req := httptest.NewRequest(http.MethodPost, "/api/instances/main/panes/3/send", body)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if len(fake.sendCalls) != 2 {
		t.Fatalf("send calls = %#v", fake.sendCalls)
	}
	if fake.sendCalls[0].text != "分析项目目录" {
		t.Fatalf("text = %q", fake.sendCalls[0].text)
	}
	if fake.sendCalls[1].text != "\r" {
		t.Fatalf("enter text = %q", fake.sendCalls[1].text)
	}
}

func TestSendTextAllowsEnterOnly(t *testing.T) {
	srv, fake := testServer(t)
	body := bytes.NewBufferString(`{"enter":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/instances/main/panes/3/send", body)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if len(fake.sendCalls) != 1 || fake.sendCalls[0].text != "\r" {
		t.Fatalf("send calls = %#v", fake.sendCalls)
	}
}

func TestDeletePane(t *testing.T) {
	srv, fake := testServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/instances/main/panes/3", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !fake.killed || fake.lastClass != "easycodex" || fake.lastPaneID != "3" {
		t.Fatalf("unexpected delete call: %#v", fake)
	}
}

func TestSpawnUsesActivePaneWhenPaneIDIsOmitted(t *testing.T) {
	srv, fake := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/instances/main/spawn", bytes.NewBufferString(`{}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if fake.lastPaneID != "3" {
		t.Fatalf("expected active pane id 3, got %q", fake.lastPaneID)
	}
}

func TestSpawnUsesExplicitPaneID(t *testing.T) {
	srv, fake := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/instances/main/spawn", bytes.NewBufferString(`{"paneId":"4"}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if fake.lastPaneID != "4" {
		t.Fatalf("expected explicit pane id 4, got %q", fake.lastPaneID)
	}
}
