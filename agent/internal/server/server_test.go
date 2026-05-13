package server

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"easycodex-agent/internal/config"
)

type fakeWezTerm struct {
	lastClass   string
	lastPaneID  string
	lastCWD     string
	lastLines   int
	lastEscapes bool
	lastText    string
	lastNoPaste bool
	lastCommand []string
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
	fake.lastCWD = cwd
	fake.lastCommand = append([]string(nil), command...)
	return "9", nil
}

func testServer(t *testing.T) (*Server, *fakeWezTerm) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
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

func TestPairingQRSupportsLongPayload(t *testing.T) {
	srv, _ := testServer(t)
	payload := "easycodex://pair?u=https%3A%2F%2F7fb07pk68535.vicp.fun%2Fapi%2Fmobile-pair%3Fcode%3Dfd44d8bc713c%26baseUrl%3Dhttps%253A%252F%252F7fb07pk68535.vicp.fun"
	req := httptest.NewRequest(http.MethodGet, "/api/pairing/qr.svg?data="+url.QueryEscape(payload), nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.Contains(contentType, "image/svg+xml") {
		t.Fatalf("unexpected content type: %s", contentType)
	}
	if body := rec.Body.String(); !strings.Contains(body, "<svg ") || !strings.Contains(body, "<rect ") {
		t.Fatalf("unexpected svg body: %s", body)
	}
}

func TestMobilePairUsesExplicitBaseURL(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/mobile-pair?code="+srv.mobilePairCode()+"&baseUrl=https%3A%2F%2Fpublic.example.test", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			BaseURL string `json:"baseUrl"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !payload.OK || payload.Data.BaseURL != "https://public.example.test" {
		t.Fatalf("unexpected mobile pair base url: %#v", payload)
	}
}

func TestMobilePairUsesForwardedProto(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/mobile-pair?code="+srv.mobilePairCode(), nil)
	req.Host = "public.example.test"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			BaseURL string `json:"baseUrl"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !payload.OK || payload.Data.BaseURL != "https://public.example.test" {
		t.Fatalf("unexpected mobile pair base url: %#v", payload)
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
		"displayName":"Office PC",
		"publicBaseUrl":"http://100.64.1.2:8765",
		"commandTimeoutSeconds":5,
		"autoScrollTerminal":false,
		"terminalRetentionLines":1200,
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
	if !found || loaded.Token != "new-secret" || !loaded.RegenerateTokenOnStart || loaded.AutoScrollTerminal || loaded.TerminalRetentionLines != 1200 || loaded.DisplayName != "Office PC" || loaded.PublicBaseURL != "http://100.64.1.2:8765" {
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

func TestSaveCodexSessionTitleWritesConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cfg := config.Defaults()
	cfg.Listen = "127.0.0.1:0"
	cfg.Root = t.TempDir()
	cfg.Token = "secret"
	path := filepath.Join(t.TempDir(), "config.json")
	fake := &fakeWezTerm{}
	srv, err := NewWithConfigPath(cfg, path, fake, nil)
	if err != nil {
		t.Fatal(err)
	}

	sessionID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	req := httptest.NewRequest(http.MethodPut, "/api/codex/sessions/"+sessionID+"/title", bytes.NewBufferString(`{"title":" Build fix "}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	loaded, found, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !found || loaded.CodexSessionTitles[sessionID] != "Build fix" {
		t.Fatalf("unexpected saved titles: found=%v cfg=%#v", found, loaded.CodexSessionTitles)
	}

	deleteReq := httptest.NewRequest(http.MethodPut, "/api/codex/sessions/"+sessionID+"/title", bytes.NewBufferString(`{"title":""}`))
	deleteReq.Header.Set("Authorization", "Bearer secret")
	deleteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d body = %s", deleteRec.Code, deleteRec.Body.String())
	}
	loaded, _, err = config.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if _, ok := loaded.CodexSessionTitles[sessionID]; ok {
		t.Fatalf("expected title deletion: %#v", loaded.CodexSessionTitles)
	}
}

func TestSettingsIncludesVersion(t *testing.T) {
	previous := AppVersion
	AppVersion = "1.2.3-test"
	t.Cleanup(func() { AppVersion = previous })
	srv, _ := testServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || payload.Data.Version != "1.2.3-test" {
		t.Fatalf("unexpected settings payload: %#v", payload)
	}

	pageReq := httptest.NewRequest(http.MethodGet, "/settings", nil)
	pageReq.RemoteAddr = "127.0.0.1:12345"
	pageRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(pageRec, pageReq)
	if pageRec.Code != http.StatusOK {
		t.Fatalf("page status = %d body = %s", pageRec.Code, pageRec.Body.String())
	}
	body := pageRec.Body.String()
	if !strings.Contains(body, "Current version") || !strings.Contains(body, `id="version"`) {
		t.Fatalf("expected version field in settings page: %s", body)
	}
	if !strings.Contains(body, `id="uiLanguage"`) ||
		!strings.Contains(body, `id="displayName"`) ||
		!strings.Contains(body, "Language") ||
		strings.Contains(body, `class="lang-switch"`) {
		t.Fatalf("expected language selector only in settings form: %s", body)
	}
	if !strings.Contains(body, `href="https://github.com/laomoi-cpu/EasyCodex"`) ||
		!strings.Contains(body, `class="github-link"`) {
		t.Fatalf("expected GitHub link in settings page: %s", body)
	}
	if !strings.Contains(body, `class="version-badge"`) ||
		!strings.Contains(body, "v1.2.3-test") {
		t.Fatalf("expected version badge in settings page: %s", body)
	}
	if !strings.Contains(body, "Check Update") ||
		!strings.Contains(body, "/api/update/check") ||
		!strings.Contains(body, "/api/update/apply") ||
		!strings.Contains(body, "/api/update/status") ||
		!strings.Contains(body, `id="useGitHubProxy" type="checkbox" checked`) ||
		!strings.Contains(body, "useGitHubProxy:$('useGitHubProxy').checked") ||
		!strings.Contains(body, `id="updateProgressBar"`) {
		t.Fatalf("expected update controls in settings page: %s", body)
	}
	if !strings.Contains(body, `id="lanPromptShown"`) ||
		!strings.Contains(body, "maybePromptLANListen") ||
		!strings.Contains(body, "/api/restart") {
		t.Fatalf("expected LAN listen prompt in settings page: %s", body)
	}
	if !strings.Contains(body, `id="autoScrollTerminal"`) ||
		!strings.Contains(body, "autoScrollTerminal:$('autoScrollTerminal').checked") ||
		!strings.Contains(body, "$('autoScrollTerminal').checked=c.autoScrollTerminal!==false") {
		t.Fatalf("expected terminal auto scroll setting in settings page: %s", body)
	}
	if !strings.Contains(body, `id="terminalRetentionLines"`) ||
		!strings.Contains(body, "terminalRetentionLines:parseInt($('terminalRetentionLines').value,10)||1000") ||
		!strings.Contains(body, "$('terminalRetentionLines').value=c.terminalRetentionLines||1000") {
		t.Fatalf("expected terminal retention setting in settings page: %s", body)
	}
}

func TestMachinePageTitleIncludesHostPrefix(t *testing.T) {
	if got := machinePageTitle("DEV-PC", "Settings"); got != "DEV-PC - EasyCodex Settings" {
		t.Fatalf("title = %q", got)
	}
	if got := machinePageTitle("", "Status"); got != "EasyCodex Status" {
		t.Fatalf("fallback title = %q", got)
	}
	if got := effectiveMachineName(config.Config{DisplayName: "Office PC"}); got != "Office PC" {
		t.Fatalf("effective name = %q", got)
	}
}

func TestAppConfigIncludesMachineName(t *testing.T) {
	cfg := config.Defaults()
	cfg.Listen = "127.0.0.1:0"
	cfg.Token = "secret"
	cfg.DisplayName = "Office PC"
	cfg.AutoScrollTerminal = false
	cfg.TerminalRetentionLines = 1200
	srv, err := New(cfg, &fakeWezTerm{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			MachineName            string `json:"machineName"`
			AutoScrollTerminal     bool   `json:"autoScrollTerminal"`
			TerminalRetentionLines int    `json:"terminalRetentionLines"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || payload.Data.MachineName != "Office PC" || payload.Data.AutoScrollTerminal || payload.Data.TerminalRetentionLines != 1200 {
		t.Fatalf("unexpected config payload: %#v", payload)
	}
}

func TestGitHubProxyURL(t *testing.T) {
	raw := "https://github.com/laomoi-cpu/EasyCodex/releases/download/v0.0.14/EasyCodex-0.0.14.patch.zip"
	want := "https://gh-proxy.org/" + raw
	if got := githubProxyURL(raw); got != want {
		t.Fatalf("proxy url = %q", got)
	}
	if got := githubProxyURL(want); got != want {
		t.Fatalf("already proxied url = %q", got)
	}
	if got := githubProxyURL("EasyCodex-0.0.14.patch.zip"); got != "EasyCodex-0.0.14.patch.zip" {
		t.Fatalf("relative url = %q", got)
	}
}

func TestSettingsLanguageSwitchPersistsConfig(t *testing.T) {
	cfg := config.Defaults()
	cfg.Listen = "127.0.0.1:0"
	cfg.Root = `D:\EasyCodex`
	cfg.Token = "secret"
	cfg.UILanguage = "en"
	path := filepath.Join(t.TempDir(), "config.json")
	fake := &fakeWezTerm{}
	srv, err := NewWithConfigPath(cfg, path, fake, nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/settings?lang=zh", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<html lang="zh">`) ||
		!strings.Contains(body, `id="uiLanguage"`) ||
		!strings.Contains(body, `currentUILanguage = "zh"`) ||
		!strings.Contains(body, "界面语言") {
		t.Fatalf("expected Chinese settings page: %s", body)
	}
	loaded, found, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.UILanguage != "zh" {
		t.Fatalf("expected persisted zh language, found=%v cfg=%#v", found, loaded)
	}
}

func TestUpdateCheckReportsNewRelease(t *testing.T) {
	previousVersion := AppVersion
	AppVersion = "0.0.7"
	t.Cleanup(func() { AppVersion = previousVersion })
	releaseServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name":     "v0.0.8",
			"html_url":     "https://github.com/laomoi-cpu/EasyCodex/releases/tag/v0.0.8",
			"published_at": "2026-05-11T00:00:00Z",
			"assets": []map[string]any{
				{"name": "EasyCodex-0.0.8.patch.zip", "browser_download_url": "https://example.com/EasyCodex-0.0.8.patch.zip"},
				{"name": "EasyCodex-0.0.8.zip", "browser_download_url": "https://example.com/EasyCodex-0.0.8.zip"},
			},
		})
	}))
	defer releaseServer.Close()
	previousURL := githubLatestReleaseURL
	previousClient := updateHTTPClient
	githubLatestReleaseURL = releaseServer.URL
	updateHTTPClient = releaseServer.Client()
	t.Cleanup(func() {
		githubLatestReleaseURL = previousURL
		updateHTTPClient = previousClient
	})
	srv, _ := testServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/update/check", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			CurrentVersion string `json:"currentVersion"`
			LatestVersion  string `json:"latestVersion"`
			CanUpdate      bool   `json:"canUpdate"`
			UpToDate       bool   `json:"upToDate"`
			ZipURL         string `json:"zipUrl"`
			PackageName    string `json:"packageName"`
			PackageKind    string `json:"packageKind"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || payload.Data.CurrentVersion != "0.0.7" || payload.Data.LatestVersion != "0.0.8" ||
		!payload.Data.CanUpdate || payload.Data.UpToDate || payload.Data.ZipURL != "https://example.com/EasyCodex-0.0.8.patch.zip" ||
		payload.Data.PackageName != "EasyCodex-0.0.8.patch.zip" || payload.Data.PackageKind != "patch" {
		t.Fatalf("unexpected update payload: %#v", payload)
	}
}

func TestUpdateCheckRequiresAuthWhenRemote(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/update/check", nil)
	req.RemoteAddr = "192.168.1.20:12345"
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestDownloadFileResumesPartialFile(t *testing.T) {
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "EasyCodex.patch.zip")
	if err := os.WriteFile(dst+".tmp", []byte("hello "), 0644); err != nil {
		t.Fatal(err)
	}
	var finalWritten int64
	var finalTotal int64
	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=6-" {
			t.Fatalf("Range = %q", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Range", "bytes 6-10/11")
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("world"))
	}))
	defer downloadServer.Close()
	previousClient := updateDownloadHTTPClient
	updateDownloadHTTPClient = downloadServer.Client()
	t.Cleanup(func() { updateDownloadHTTPClient = previousClient })

	err := downloadFile(context.Background(), downloadServer.URL, dst, func(written, total int64) {
		finalWritten = written
		finalTotal = total
	})

	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello world" {
		t.Fatalf("content = %q", content)
	}
	if _, err := os.Stat(dst + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary file should be renamed, stat err = %v", err)
	}
	if finalWritten != 11 || finalTotal != 11 {
		t.Fatalf("progress = %d/%d", finalWritten, finalTotal)
	}
}

func TestDownloadFileRetriesAfterServerError(t *testing.T) {
	tmpDir := t.TempDir()
	dst := filepath.Join(tmpDir, "EasyCodex.patch.zip")
	attempts := 0
	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "temporary failure", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer downloadServer.Close()
	previousClient := updateDownloadHTTPClient
	updateDownloadHTTPClient = downloadServer.Client()
	t.Cleanup(func() { updateDownloadHTTPClient = previousClient })

	err := downloadFile(context.Background(), downloadServer.URL, dst, nil)

	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "ok" || attempts != 2 {
		t.Fatalf("content=%q attempts=%d", content, attempts)
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
	if !strings.Contains(body, "/terminal#baseUrl=") ||
		!strings.Contains(body, "PC / phone browser access") ||
		!strings.Contains(body, `class="link-field"`) ||
		!strings.Contains(body, "Browser terminal QR code") {
		t.Fatalf("expected browser terminal pairing card: %s", body)
	}
}

func TestTerminalPageIsAvailableRemotely(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/terminal", nil)
	req.RemoteAddr = "192.168.1.20:12345"
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Browser Terminal") ||
		!strings.Contains(body, "terminalApp") ||
		strings.Contains(body, `<header class="topbar">`) ||
		strings.Contains(body, `href="/settings"`) ||
		strings.Contains(body, `id="uiLanguage"`) ||
		strings.Contains(body, `class="lang-switch"`) ||
		!strings.Contains(body, `class="page-terminal"`) ||
		!strings.Contains(body, ".page-terminal main{max-width:none;height:100dvh") ||
		!strings.Contains(body, ".pane-last{display:block") ||
		!strings.Contains(body, ".pane-state{display:inline-flex") ||
		!strings.Contains(body, ".pane-state.confirm{") ||
		!strings.Contains(body, ".pane-confirm-mark{display:inline-grid") ||
		!strings.Contains(body, ".pane-notify-dot{display:inline-block") ||
		!strings.Contains(body, ".page-terminal .terminal-output{min-height:62dvh") ||
		!strings.Contains(body, ".page-terminal .send-row{position:sticky") ||
		!strings.Contains(body, `id="androidBridgeBar"`) ||
		!strings.Contains(body, `id="androidBridgeScan"`) ||
		!strings.Contains(body, `.page-terminal.android-webview .terminal-statusbar{display:none!important}`) ||
		!strings.Contains(body, "function setupAndroidBridgeChrome()") ||
		!strings.Contains(body, "window.easycodexSetupAndroidBridge = setupAndroidBridgeChrome") ||
		!strings.Contains(body, "$('androidBridgeScan').onclick = () => callAndroidBridge('scanPairing')") ||
		!strings.Contains(body, `id="mobileInputDialog"`) ||
		!strings.Contains(body, `id="mobileCommandInput"`) ||
		!strings.Contains(body, `id="mobileAddAttachment"`) ||
		!strings.Contains(body, `id="mobileAttachmentPanel"`) ||
		!strings.Contains(body, `id="customSessionTitle"`) ||
		!strings.Contains(body, `id="saveSessionTitle"`) ||
		!strings.Contains(body, ".mobile-input-dialog textarea{min-height:160px") ||
		!strings.Contains(body, "function openMobileInputDialog()") ||
		!strings.Contains(body, "function syncMobileCommandInputMode()") ||
		!strings.Contains(body, "function renderAttachmentPanel(panelId, summaryId, listId)") ||
		!strings.Contains(body, "function shouldInsertDroppedFilePaths()") ||
		!strings.Contains(body, "return isLocalBrowser() && !isMobileInputMode()") ||
		!strings.Contains(body, "function droppedFilePaths(dataTransfer)") ||
		!strings.Contains(body, "function insertCommandInputText(text)") ||
		!strings.Contains(body, "function fileURIToLocalPath(uri)") ||
		!strings.Contains(body, "insertCommandInputText(paths.map(pathForCommandInput).join(' '))") ||
		!strings.Contains(body, "commandDrafts: {}") ||
		!strings.Contains(body, "outputBuffers: {}") ||
		!strings.Contains(body, "function saveCommandDraft()") ||
		!strings.Contains(body, "function restoreCommandDraft(paneId)") ||
		!strings.Contains(body, "$('commandInput').addEventListener('input', saveCommandDraft)") ||
		!strings.Contains(body, "$('commandInput').addEventListener('pointerdown', handleCommandInputIntent)") ||
		!strings.Contains(body, "$('mobileInputSend').onclick = () => sendMobileInput().catch") ||
		!strings.Contains(body, "$('mobileAddAttachment').onclick = () => $('attachmentInput').click()") ||
		!strings.Contains(body, ".key-panel[hidden]{display:none!important}") ||
		!strings.Contains(body, ".page-terminal .terminal-app[hidden],.page-terminal .terminal-connect[hidden]{display:none!important}") ||
		!strings.Contains(body, "font-size:var(--terminal-font-size,14px)") ||
		!strings.Contains(body, `"JetBrains Mono","Cascadia Mono",Consolas`) ||
		!strings.Contains(body, "const ansiColors = [0x000000,0xcc5555") ||
		!strings.Contains(body, ".page-terminal .terminal-sidebar{border:0;border-radius:0;box-shadow:none;max-height:none;min-height:0;padding:6px;background:#1f1f1f;display:grid;grid-template-columns:auto minmax(0,1fr)") ||
		!strings.Contains(body, ".page-terminal .pane-list{display:flex;gap:5px;min-width:0;overflow-x:auto;overflow-y:hidden") ||
		!strings.Contains(body, ".page-terminal #newSession{width:32px") ||
		!strings.Contains(body, `id="toggleFullscreen"`) ||
		!strings.Contains(body, ".terminal-app:fullscreen") ||
		!strings.Contains(body, `id="attachmentPanel"`) ||
		!strings.Contains(body, "function apiForm(path, formData)") ||
		!strings.Contains(body, "function uploadPendingAttachments()") ||
		!strings.Contains(body, "function handlePaste(event)") ||
		!strings.Contains(body, "function toggleFullscreen()") ||
		!strings.Contains(body, "await lockPortraitFullscreen()") ||
		!strings.Contains(body, "orientation.lock('portrait')") ||
		!strings.Contains(body, "document.addEventListener('fullscreenchange'") ||
		!strings.Contains(body, "addEventListener('drop', handleDrop)") ||
		!strings.Contains(body, `id="connectionDialog"`) ||
		!strings.Contains(body, `id="dialogAutoScrollTerminal"`) ||
		!strings.Contains(body, `id="terminalCheckUpdate"`) ||
		!strings.Contains(body, "function openConnectionDialog()") ||
		!strings.Contains(body, "function checkTerminalServerUpdate()") ||
		!strings.Contains(body, "/api/update/apply") ||
		!strings.Contains(body, "$('editConnection').onclick = () => openConnectionDialog()") ||
		!strings.Contains(body, "$('terminalCheckUpdate').onclick = () => checkTerminalServerUpdate()") ||
		!strings.Contains(body, "function fitTerminalFont()") ||
		!strings.Contains(body, "function applySessionsData(data)") ||
		!strings.Contains(body, "store.getItem('easycodex.autoScrollTerminal') !== 'false'") ||
		!strings.Contains(body, "params.get('autoScrollTerminal')") ||
		!strings.Contains(body, "if (store.getItem('easycodex.autoScrollTerminal') === null) state.autoScrollTerminal = cfg.autoScrollTerminal !== false") ||
		!strings.Contains(body, "state.terminalRetentionLines = normalizeRetentionLines(cfg.terminalRetentionLines)") ||
		!strings.Contains(body, "store.setItem('easycodex.autoScrollTerminal', state.autoScrollTerminal ? 'true' : 'false')") ||
		!strings.Contains(body, "if (state.autoScrollTerminal) output.scrollTop = output.scrollHeight") ||
		!strings.Contains(body, "function mergeTerminalBuffer(current, snapshot)") ||
		!strings.Contains(body, "function trimTerminalLines(text, limit)") ||
		!strings.Contains(body, "snapshot?lines=' + encodeURIComponent(snapshotLineCount())") ||
		!strings.Contains(body, "state.outputBuffers[paneId] = trimTerminalLines(merged, state.terminalRetentionLines)") ||
		!strings.Contains(body, "state.paneWorking[paneId] === true && !isWorking") ||
		!strings.Contains(body, "state.confirmCount = Number(data.confirmCount") ||
		!strings.Contains(body, "function clearPaneNotify(paneId, rerender)") ||
		!strings.Contains(body, "const notify = paneHasNotify(pane.paneId)") ||
		!strings.Contains(body, "function updateDocumentTitle()") ||
		!strings.Contains(body, "baseDocumentTitle") ||
		!strings.Contains(body, "function notifyAndroidWorkingCount()") ||
		!strings.Contains(body, "window.EasyCodexAndroid.updateWorkingCount") ||
		!strings.Contains(body, "parts.push((i18n.confirm || 'Confirm') + ' ' + state.confirmCount)") ||
		!strings.Contains(body, "mark.className = 'pane-confirm-mark'") ||
		!strings.Contains(body, "state.confirmCount > 0 || paneNotifyCount() > 0") ||
		!strings.Contains(body, "function saveCustomSessionTitle()") ||
		!strings.Contains(body, "/api/codex/sessions/' + encodeURIComponent(pane.codexSessionId) + '/title") ||
		!strings.Contains(body, "return pane.displayTitle || pane.customTitle || pane.title || pane.cwd || ''") ||
		!strings.Contains(body, "session.displayTitle || session.customTitle || session.summary || session.id") ||
		!strings.Contains(body, "function paneNotifyCount()") ||
		!strings.Contains(body, "parts.push('Notify ' + notifyCount)") ||
		!strings.Contains(body, "return terminalMachineName ? status + ' - ' + terminalMachineName : status") ||
		!strings.Contains(body, "function snapshotPollInterval()") ||
		!strings.Contains(body, "return isLocalBrowser() ? 300 : 1000") ||
		!strings.Contains(body, "return isLocalBrowser() ? 300 : 2000") ||
		!strings.Contains(body, "function markPaneInput(text)") ||
		!strings.Contains(body, "recordInput: !!recordInput") ||
		!strings.Contains(body, "row.onpointerdown = event =>") ||
		!strings.Contains(body, "const changed = state.paneId !== paneId") ||
		!strings.Contains(body, "await sendRaw(appendAttachmentPaths(text, uploads), enter, true)") ||
		!strings.Contains(body, "clearPaneNotify(state.paneId, true)") ||
		!strings.Contains(body, "sendRaw(value[0], value[1], false)") ||
		!strings.Contains(body, "refreshPaneList().catch(() => {})") ||
		!strings.Contains(body, "function terminalShortcutFromEvent(event)") ||
		!strings.Contains(body, "return hasBrowserTextSelection() ? '' : 'ctrlc'") ||
		!strings.Contains(body, "function terminalKeyboardReady()") ||
		!strings.Contains(body, "function hasBrowserTextSelection()") ||
		!strings.Contains(body, "window.getSelection && window.getSelection()") ||
		!strings.Contains(body, "document.body.classList.contains('page-terminal')") ||
		!strings.Contains(body, "event.key === 'Tab' && event.shiftKey") ||
		!strings.Contains(body, "document.addEventListener('keydown'") ||
		!strings.Contains(body, "setInterval(() =>") ||
		!strings.Contains(body, "function setKeyPanel(show)") ||
		!strings.Contains(body, "$('toggleKeys').onclick = () => setKeyPanel($('keyPanel').hidden)") ||
		!strings.Contains(body, `id="spawnDialog"`) ||
		!strings.Contains(body, "function openSpawnDialog(cwd)") ||
		!strings.Contains(body, "$('newSession').onclick = () => openSpawnDialog()") ||
		!strings.Contains(body, "/api/codex/sessions?limit=20") ||
		!strings.Contains(body, "['cmd.exe','/k','codex','resume']") ||
		!strings.Contains(body, "function selectedCodexSession()") ||
		!strings.Contains(body, "const selectedSession = selectedCodexSession()") ||
		!strings.Contains(body, "if (options.codexSessionId) body.codexSessionId = options.codexSessionId") ||
		!strings.Contains(body, "if (!selectedCodexSession()) state.selectedCodexSessionId = ''") ||
		!strings.Contains(body, "if (session.cwd) $('spawnCwd').value = spawnCwdFromValue(session.cwd)") ||
		!strings.Contains(body, "snapshot?lines=") ||
		strings.Contains(body, `id="refreshSessions"`) ||
		strings.Contains(body, "$('refreshSessions')") {
		t.Fatalf("unexpected terminal page: %s", body)
	}
}

func TestReadCodexSessionItemUsesUserMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-2026-05-11T00-00-00-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee.jsonl")
	data := strings.Join([]string{
		`{"type":"session_meta","payload":{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","cwd":"D:\\mgame","timestamp":"2026-05-11T08:00:00Z"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"internal instruction should be ignored"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>\n  <cwd>D:\\mgame</cwd>\n</environment_context>"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hi，分析一下项目"}]}}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	item, ok := readCodexSessionItem(path)
	if !ok {
		t.Fatal("expected codex session item")
	}
	if item.ID != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" ||
		item.CWD != `D:\mgame` ||
		item.Timestamp != "2026-05-11T08:00:00Z" ||
		item.Summary != "hi，分析一下项目" {
		t.Fatalf("unexpected session item: %#v", item)
	}
}

func writeCodexSessionForTest(t *testing.T, home, sessionID, cwd string) string {
	t.Helper()
	dir := filepath.Join(home, ".codex", "sessions", "2026", "05", "13")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rollout-2026-05-13T00-00-00-"+sessionID+".jsonl")
	cwdJSON, err := json.Marshal(cwd)
	if err != nil {
		t.Fatal(err)
	}
	data := `{"type":"session_meta","payload":{"id":"` + sessionID + `","cwd":` + string(cwdJSON) + `,"timestamp":"2026-05-13T08:00:00Z"}}` + "\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConsoleNavOmitsTerminalAndIncludesConnections(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, `href="/terminal">Terminal`) {
		t.Fatalf("terminal link should not be in console nav: %s", body)
	}
	if !strings.Contains(body, `href="/connections">Connections`) {
		t.Fatalf("connections link should be in console nav: %s", body)
	}
	if !strings.Contains(body, `id="runNetworkTests"`) ||
		!strings.Contains(body, "/api/network-tests") ||
		!strings.Contains(body, "HTTP Service Test") {
		t.Fatalf("network test controls should be in status page: %s", body)
	}
}

func TestNetworkTestsChecksHealthEndpoint(t *testing.T) {
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		writeOK(w, http.StatusOK, map[string]any{
			"service":    "easycodex-agent",
			"lanEnabled": true,
		})
	}))
	defer health.Close()
	cfg := config.Defaults()
	cfg.Listen = strings.TrimPrefix(health.URL, "http://")
	cfg.Root = `D:\EasyCodex`
	cfg.Token = "secret"
	fake := &fakeWezTerm{}
	srv, err := New(cfg, fake, nil)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/network-tests", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Results []networkTestResult `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || len(payload.Data.Results) != 1 || !payload.Data.Results[0].OK || payload.Data.Results[0].Service != "easycodex-agent" {
		t.Fatalf("unexpected network test payload: %#v", payload)
	}
}

func TestConnectionsTracksAuthenticatedClients(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	req.RemoteAddr = "192.168.1.50:4567"
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("User-Agent", "EasyCodex-Android/1")
	req.Header.Set("X-EasyCodex-Client-ID", "android:test-device")
	req.Header.Set("X-EasyCodex-Client-Kind", "android")
	req.Header.Set("X-EasyCodex-Client-Name", "Android App")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/connections", nil)
	listReq.RemoteAddr = "127.0.0.1:12345"
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", listRec.Code, listRec.Body.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Connections []clientConnection `json:"connections"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || len(payload.Data.Connections) != 1 {
		t.Fatalf("unexpected connections payload: %#v", payload)
	}
	item := payload.Data.Connections[0]
	if item.ID != "android:test-device" || item.Kind != "Android App" || item.RemoteAddr != "192.168.1.50" || item.LastPath != "/api/instances" || item.Requests != 1 {
		t.Fatalf("unexpected connection item: %#v", item)
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
			Instance     string          `json:"instance"`
			WorkingCount int             `json:"workingCount"`
			ConfirmCount int             `json:"confirmCount"`
			Windows      []windowSession `json:"windows"`
			Panes        []paneSession   `json:"panes"`
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
	if payload.Data.WorkingCount != 0 {
		t.Fatalf("working count = %d", payload.Data.WorkingCount)
	}
	if payload.Data.ConfirmCount != 0 {
		t.Fatalf("confirm count = %d", payload.Data.ConfirmCount)
	}
	if len(payload.Data.Panes) != 2 || payload.Data.Panes[0].PaneID != "3" || payload.Data.Panes[1].PaneID != "4" {
		t.Fatalf("unexpected panes: %#v", payload.Data.Panes)
	}
}

func TestSessionsAppliesCodexTitleFromResumeCommand(t *testing.T) {
	srv, _ := testServer(t)
	sessionID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	cfg := srv.configSnapshot()
	cfg.CodexSessionTitles = map[string]string{sessionID: "Build fix"}
	srv.setConfig(cfg)

	sendReq := httptest.NewRequest(http.MethodPost, "/api/instances/main/panes/3/send", bytes.NewBufferString(`{"text":"codex resume `+sessionID+`","noPaste":true}`))
	sendReq.Header.Set("Authorization", "Bearer secret")
	sendRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(sendRec, sendReq)
	if sendRec.Code != http.StatusOK {
		t.Fatalf("send status = %d body = %s", sendRec.Code, sendRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/instances/main/sessions", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sessions status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Panes []paneSession `json:"panes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !payload.OK || payload.Data.Panes[0].CodexSessionID != sessionID || payload.Data.Panes[0].CustomTitle != "Build fix" || payload.Data.Panes[0].DisplayTitle != "Build fix" {
		t.Fatalf("unexpected panes: %#v", payload.Data.Panes)
	}
}

func TestSessionsDoesNotInferCodexTitleFromRecentCWD(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	sessionID := "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"
	writeCodexSessionForTest(t, home, sessionID, `D:\mgame`)
	cfg := config.Config{
		Listen:             "127.0.0.1:0",
		Root:               `D:\EasyCodex`,
		Token:              "secret",
		CodexSessionTitles: map[string]string{sessionID: "Mobile polish"},
		Instances: []config.Instance{
			{ID: "main", Name: "main", Class: "easycodex"},
		},
	}
	fake := &fakeWezTerm{}
	srv, err := New(cfg, fake, nil)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/instances/main/sessions", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sessions status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Panes []paneSession `json:"panes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !payload.OK || payload.Data.Panes[0].CodexSessionID != "" || payload.Data.Panes[0].CustomTitle != "" || payload.Data.Panes[0].DisplayTitle != "" {
		t.Fatalf("unexpected panes: %#v", payload.Data.Panes)
	}
}

func TestCodexSessionsReturnCustomTitles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	sessionID := "cccccccc-dddd-eeee-ffff-000000000000"
	writeCodexSessionForTest(t, home, sessionID, `D:\mgame`)
	cfg := config.Config{
		Listen:             "127.0.0.1:0",
		Root:               `D:\EasyCodex`,
		Token:              "secret",
		CodexSessionTitles: map[string]string{sessionID: "Release followup"},
		Instances: []config.Instance{
			{ID: "main", Name: "main", Class: "easycodex"},
		},
	}
	fake := &fakeWezTerm{}
	srv, err := New(cfg, fake, nil)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/codex/sessions?limit=20", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Sessions []codexSessionItem `json:"sessions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !payload.OK || len(payload.Data.Sessions) != 1 || payload.Data.Sessions[0].CustomTitle != "Release followup" || payload.Data.Sessions[0].DisplayTitle != "Release followup" {
		t.Fatalf("unexpected sessions: %#v", payload.Data.Sessions)
	}
}

func TestNormalizeSessionsMarksWorkingPanes(t *testing.T) {
	tree, err := normalizeSessions("main", json.RawMessage(`[
		{"window_id":1,"window_title":"EasyCodex (1 working) - 3 sessions","tab_id":1,"tab_title":"","pane_id":1,"title":"cmd.exe","cwd":"file:///D:/idle/","workspace":"default","size":{"cols":80,"rows":24}},
		{"window_id":1,"window_title":"EasyCodex","tab_id":2,"tab_title":"codex thinking","pane_id":2,"title":"mgame","cwd":"file:///D:/mgame/","workspace":"default","size":{"cols":80,"rows":24}},
		{"window_id":1,"window_title":"EasyCodex","tab_id":3,"tab_title":"","pane_id":3,"title":"\u2838 EasyTerm","cwd":"file:///D:/EasyTerm/","workspace":"default","size":{"cols":80,"rows":24}}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if tree.WorkingCount != 2 {
		t.Fatalf("working count = %d", tree.WorkingCount)
	}
	if len(tree.Panes) != 3 {
		t.Fatalf("panes = %#v", tree.Panes)
	}
	if tree.Panes[0].IsWorking {
		t.Fatalf("window title should not mark idle pane working: %#v", tree.Panes[0])
	}
	if !tree.Panes[1].IsWorking || !tree.Panes[2].IsWorking {
		t.Fatalf("expected keyword and spinner panes working: %#v", tree.Panes)
	}
	if !tree.Windows[0].Tabs[1].Panes[0].IsWorking || !tree.Windows[0].Tabs[2].Panes[0].IsWorking {
		t.Fatalf("nested panes missing working state: %#v", tree.Windows[0].Tabs)
	}
}

func TestNormalizeSessionsMarksConfirmPanes(t *testing.T) {
	tree, err := normalizeSessions("main", json.RawMessage(`[
		{"window_id":1,"window_title":"EasyCodex","tab_id":1,"tab_title":"","pane_id":1,"title":"? EasyCodex","cwd":"file:///D:/EasyCodex/","workspace":"default","size":{"cols":80,"rows":24}},
		{"window_id":1,"window_title":"EasyCodex","tab_id":2,"tab_title":"waiting for input","pane_id":2,"title":"mgame","cwd":"file:///D:/mgame/","workspace":"default","size":{"cols":80,"rows":24}},
		{"window_id":1,"window_title":"EasyCodex","tab_id":3,"tab_title":"","pane_id":3,"title":"plan notes","cwd":"file:///D:/plan/","workspace":"default","size":{"cols":80,"rows":24}},
		{"window_id":1,"window_title":"EasyCodex","tab_id":4,"tab_title":"","pane_id":4,"title":"\u2838 EasyTerm confirm","cwd":"file:///D:/work/","workspace":"default","size":{"cols":80,"rows":24}}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if tree.ConfirmCount != 2 {
		t.Fatalf("confirm count = %d panes=%#v", tree.ConfirmCount, tree.Panes)
	}
	if !tree.Panes[0].IsConfirm || !tree.Panes[1].IsConfirm {
		t.Fatalf("expected confirm panes: %#v", tree.Panes)
	}
	if tree.Panes[2].IsConfirm {
		t.Fatalf("plain plan title should not be confirm: %#v", tree.Panes[2])
	}
	if !tree.Panes[3].IsWorking || tree.Panes[3].IsConfirm {
		t.Fatalf("working should win over confirm: %#v", tree.Panes[3])
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

func TestUploadAttachmentsSavesFiles(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		Listen: "127.0.0.1:0",
		Root:   root,
		Token:  "secret",
		Instances: []config.Instance{
			{ID: "main", Name: "main", Class: "easycodex"},
		},
	}
	srv, err := New(cfg, &fakeWezTerm{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	body, contentType := multipartUploadBody(t, map[string]string{
		`..\bad:name?.txt`: "hello attachment",
		"screenshot.png":   "png bytes",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/instances/main/panes/3/attachments", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Attachments []attachmentUpload `json:"attachments"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || len(payload.Data.Attachments) != 2 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	for _, item := range payload.Data.Attachments {
		if !isWithinDir(filepath.Join(root, ".attachments"), item.Path) {
			t.Fatalf("path outside attachments dir: %q", item.Path)
		}
		if strings.ContainsAny(item.FileName, `<>:"/\|?*`) {
			t.Fatalf("filename was not sanitized: %q", item.FileName)
		}
		data, err := os.ReadFile(item.Path)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) == 0 || item.Size != int64(len(data)) {
			t.Fatalf("unexpected saved file: %#v len=%d", item, len(data))
		}
	}
}

func TestUploadAttachmentsRejectsOversizedFile(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{
		Listen: "127.0.0.1:0",
		Root:   root,
		Token:  "secret",
		Instances: []config.Instance{
			{ID: "main", Name: "main", Class: "easycodex"},
		},
	}
	srv, err := New(cfg, &fakeWezTerm{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	body, contentType := multipartUploadBody(t, map[string]string{
		"large.txt": strings.Repeat("x", maxAttachmentFileBytes+1),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/instances/main/panes/3/attachments", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func multipartUploadBody(t *testing.T, files map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for name, content := range files {
		part, err := writer.CreateFormFile("files", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body, writer.FormDataContentType()
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

func TestSessionsIncludeLastInputSummary(t *testing.T) {
	srv, _ := testServer(t)
	body := bytes.NewBufferString(`{"text":"12345678901234567890abcdef","noPaste":true}`)
	sendReq := httptest.NewRequest(http.MethodPost, "/api/instances/main/panes/3/send", body)
	sendReq.Header.Set("Authorization", "Bearer secret")
	sendRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(sendRec, sendReq)
	if sendRec.Code != http.StatusOK {
		t.Fatalf("send status = %d body = %s", sendRec.Code, sendRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/instances/main/sessions", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("sessions status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Panes []paneSession `json:"panes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || len(payload.Data.Panes) == 0 {
		t.Fatalf("unexpected sessions payload: %#v", payload)
	}
	if payload.Data.Panes[0].LastInput != "12345678901234567890..." || payload.Data.Panes[0].LastInputAt == "" {
		t.Fatalf("unexpected last input: %#v", payload.Data.Panes[0])
	}
}

func TestSendTextRecordInputFalseSkipsLastInput(t *testing.T) {
	srv, _ := testServer(t)
	body := bytes.NewBufferString(`{"text":"manual shortcut","noPaste":true,"recordInput":false}`)
	sendReq := httptest.NewRequest(http.MethodPost, "/api/instances/main/panes/3/send", body)
	sendReq.Header.Set("Authorization", "Bearer secret")
	sendRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(sendRec, sendReq)
	if sendRec.Code != http.StatusOK {
		t.Fatalf("send status = %d body = %s", sendRec.Code, sendRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/instances/main/sessions", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("sessions status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Panes []paneSession `json:"panes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || len(payload.Data.Panes) == 0 {
		t.Fatalf("unexpected sessions payload: %#v", payload)
	}
	if payload.Data.Panes[0].LastInput != "" || payload.Data.Panes[0].LastInputAt != "" {
		t.Fatalf("shortcut input should not be recorded: %#v", payload.Data.Panes[0])
	}
}

func TestSendTextControlSequenceDoesNotRecordByDefault(t *testing.T) {
	srv, _ := testServer(t)
	body := bytes.NewBufferString(`{"text":"\u001b[A","noPaste":true}`)
	sendReq := httptest.NewRequest(http.MethodPost, "/api/instances/main/panes/3/send", body)
	sendReq.Header.Set("Authorization", "Bearer secret")
	sendRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(sendRec, sendReq)
	if sendRec.Code != http.StatusOK {
		t.Fatalf("send status = %d body = %s", sendRec.Code, sendRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/instances/main/sessions", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("sessions status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Panes []paneSession `json:"panes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || len(payload.Data.Panes) == 0 {
		t.Fatalf("unexpected sessions payload: %#v", payload)
	}
	if payload.Data.Panes[0].LastInput != "" || payload.Data.Panes[0].LastInputAt != "" {
		t.Fatalf("control sequence should not be recorded: %#v", payload.Data.Panes[0])
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

func TestSpawnRecordsExplicitCodexSessionID(t *testing.T) {
	srv, fake := testServer(t)
	sessionID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	req := httptest.NewRequest(http.MethodPost, "/api/instances/main/spawn", bytes.NewBufferString(`{"cwd":"D:\\mgame","command":["cmd.exe","/k","codex","resume","`+sessionID+`"],"codexSessionId":"`+sessionID+`"}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if strings.Join(fake.lastCommand, " ") != "cmd.exe /k codex resume "+sessionID {
		t.Fatalf("unexpected command: %#v", fake.lastCommand)
	}
	inputs := srv.paneInputsSnapshot("main")
	if inputs["9"].CodexSessionID != sessionID {
		t.Fatalf("codex session id was not recorded: %#v", inputs)
	}
}
