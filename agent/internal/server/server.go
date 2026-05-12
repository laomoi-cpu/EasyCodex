package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"easycodex-agent/internal/config"
	"easycodex-agent/internal/netinfo"
	"easycodex-agent/internal/qr"
)

var AppVersion = "dev"

const (
	maxAttachmentFileBytes  = 25 << 20
	maxAttachmentTotalBytes = 100 << 20
)

type WezTerm interface {
	Launch(ctx context.Context, class string) (int, error)
	List(ctx context.Context, class string) (json.RawMessage, error)
	GetText(ctx context.Context, class, paneID string, lines int, escapes bool) (string, error)
	SendText(ctx context.Context, class, paneID, text string, noPaste bool) error
	KillPane(ctx context.Context, class, paneID string) error
	Spawn(ctx context.Context, class, paneID, cwd string, newWindow bool, command []string) (string, error)
}

type Server struct {
	mu         sync.RWMutex
	cfg        config.Config
	configPath string
	wezterm    WezTerm
	instances  map[string]config.Instance
	clients    map[string]clientConnection
	paneInputs map[string]paneInput
	restart    func()
	updateJob  updateJobStatus
	logger     *slog.Logger
}

type paneInput struct {
	Text      string
	UpdatedAt time.Time
}

type pairingResponse struct {
	Service     string                 `json:"service"`
	Network     netinfo.Info           `json:"network"`
	Token       string                 `json:"token"`
	Instances   []instanceResponse     `json:"instances"`
	Defaults    mobileDefaultsResponse `json:"defaults"`
	GeneratedAt string                 `json:"generatedAt"`
}

type apiResponse struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

type instanceResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Class string `json:"class"`
}

type mobileDefaultsResponse struct {
	InstanceID string   `json:"instanceId"`
	CWD        string   `json:"cwd"`
	Command    []string `json:"command"`
}

type appConfigResponse struct {
	Instances []instanceResponse     `json:"instances"`
	Defaults  mobileDefaultsResponse `json:"defaults"`
	Machine   string                 `json:"machineName"`
}

type settingsResponse struct {
	Config          config.Config `json:"config"`
	ConfigPath      string        `json:"configPath"`
	Network         netinfo.Info  `json:"network"`
	Version         string        `json:"version"`
	RestartRequired bool          `json:"restartRequired"`
	RestartFields   []string      `json:"restartFields,omitempty"`
}

type networkTestTarget struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type networkTestResult struct {
	Label      string `json:"label"`
	URL        string `json:"url"`
	OK         bool   `json:"ok"`
	Status     int    `json:"status,omitempty"`
	LatencyMS  int64  `json:"latencyMs"`
	Service    string `json:"service,omitempty"`
	LANEnabled *bool  `json:"lanEnabled,omitempty"`
	Error      string `json:"error,omitempty"`
	TestedAt   string `json:"testedAt"`
}

type clientConnection struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	RemoteAddr string `json:"remoteAddr"`
	UserAgent  string `json:"userAgent"`
	LastMethod string `json:"lastMethod"`
	LastPath   string `json:"lastPath"`
	FirstSeen  string `json:"firstSeen"`
	LastSeen   string `json:"lastSeen"`
	Requests   int    `json:"requests"`

	firstSeenTime time.Time
	lastSeenTime  time.Time
}

type sendTextRequest struct {
	Text             string `json:"text"`
	TextBase64       string `json:"textBase64"`
	NoPaste          bool   `json:"noPaste"`
	Enter            bool   `json:"enter"`
	EnterDelayMillis int    `json:"enterDelayMillis"`
	RecordInput      *bool  `json:"recordInput,omitempty"`
}

type attachmentUpload struct {
	OriginalName string `json:"originalName"`
	FileName     string `json:"fileName"`
	Path         string `json:"path"`
	Size         int64  `json:"size"`
	MIME         string `json:"mime,omitempty"`
}

type codexSessionItem struct {
	ID        string `json:"id"`
	CWD       string `json:"cwd,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	Summary   string `json:"summary,omitempty"`
	Path      string `json:"path,omitempty"`
}

type weztermPane struct {
	WindowID         int             `json:"window_id"`
	WindowTitle      string          `json:"window_title"`
	TabID            int             `json:"tab_id"`
	TabTitle         string          `json:"tab_title"`
	PaneID           int             `json:"pane_id"`
	Title            string          `json:"title"`
	CWD              string          `json:"cwd"`
	Workspace        string          `json:"workspace"`
	IsActive         bool            `json:"is_active"`
	IsZoomed         bool            `json:"is_zoomed"`
	CursorX          int             `json:"cursor_x"`
	CursorY          int             `json:"cursor_y"`
	CursorShape      string          `json:"cursor_shape"`
	CursorVisibility string          `json:"cursor_visibility"`
	LeftCol          int             `json:"left_col"`
	TopRow           int             `json:"top_row"`
	TTYName          *string         `json:"tty_name"`
	Size             json.RawMessage `json:"size"`
}

type sessionTree struct {
	Instance     string          `json:"instance"`
	WorkingCount int             `json:"workingCount"`
	Windows      []windowSession `json:"windows"`
	Panes        []paneSession   `json:"panes"`
}

type windowSession struct {
	WindowID  int          `json:"windowId"`
	Title     string       `json:"title"`
	Workspace string       `json:"workspace"`
	IsActive  bool         `json:"isActive"`
	Tabs      []tabSession `json:"tabs"`
}

type tabSession struct {
	TabID    int           `json:"tabId"`
	Title    string        `json:"title"`
	IsActive bool          `json:"isActive"`
	IsZoomed bool          `json:"isZoomed"`
	Panes    []paneSession `json:"panes"`
}

type paneSession struct {
	PaneID           string          `json:"paneId"`
	WindowID         int             `json:"windowId"`
	TabID            int             `json:"tabId"`
	Title            string          `json:"title"`
	CWD              string          `json:"cwd"`
	LastInput        string          `json:"lastInput,omitempty"`
	LastInputAt      string          `json:"lastInputAt,omitempty"`
	Workspace        string          `json:"workspace"`
	IsActive         bool            `json:"isActive"`
	IsWorking        bool            `json:"isWorking"`
	IsZoomed         bool            `json:"isZoomed"`
	CursorX          int             `json:"cursorX"`
	CursorY          int             `json:"cursorY"`
	CursorShape      string          `json:"cursorShape"`
	CursorVisibility string          `json:"cursorVisibility"`
	LeftCol          int             `json:"leftCol"`
	TopRow           int             `json:"topRow"`
	TTYName          *string         `json:"ttyName"`
	Size             json.RawMessage `json:"size,omitempty"`
}

type textQuery struct {
	Lines   int
	Escapes bool
}

func New(cfg config.Config, wezterm WezTerm, logger *slog.Logger) (*Server, error) {
	config.Normalize(&cfg)
	return NewWithConfigPath(cfg, filepath.Join(cfg.Root, "agent", "config.json"), wezterm, logger)
}

func NewWithConfigPath(cfg config.Config, configPath string, wezterm WezTerm, logger *slog.Logger) (*Server, error) {
	config.Normalize(&cfg)
	if err := config.Validate(cfg); err != nil {
		return nil, err
	}
	if wezterm == nil {
		return nil, errors.New("wezterm client is required")
	}
	if logger == nil {
		logger = slog.Default()
	}

	instances := make(map[string]config.Instance, len(cfg.Instances))
	for _, instance := range cfg.Instances {
		instances[instance.ID] = instance
	}
	return &Server{cfg: cfg, configPath: configPath, wezterm: wezterm, instances: instances, clients: map[string]clientConnection{}, paneInputs: map[string]paneInput{}, logger: logger}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.homePage)
	mux.HandleFunc("GET /status", s.statusPage)
	mux.HandleFunc("GET /settings", s.settingsPage)
	mux.HandleFunc("GET /connections", s.connectionsPage)
	mux.HandleFunc("GET /terminal", s.terminalPage)
	mux.HandleFunc("GET /assets/easycodex.svg", s.easycodexIcon)
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/pairing", s.pairing)
	mux.HandleFunc("GET /pairing", s.pairingPage)
	mux.HandleFunc("GET /api/pairing/qr.svg", s.pairingQR)
	mux.HandleFunc("GET /api/mobile-pair", s.mobilePair)
	mux.HandleFunc("GET /api/config", s.auth(s.appConfig))
	mux.HandleFunc("GET /api/codex/sessions", s.auth(s.codexSessions))
	mux.HandleFunc("GET /api/settings", s.localOnly(s.settings))
	mux.HandleFunc("GET /api/connections", s.localOnly(s.connections))
	mux.HandleFunc("GET /api/network-tests", s.localOnly(s.networkTests))
	mux.HandleFunc("GET /api/update/check", s.localOnly(s.checkUpdate))
	mux.HandleFunc("GET /api/update/status", s.localOnly(s.updateStatus))
	mux.HandleFunc("POST /api/update/apply", s.localOnly(s.applyUpdate))
	mux.HandleFunc("POST /api/restart", s.localOnly(s.restartAgent))
	mux.HandleFunc("POST /api/settings", s.localOnly(s.saveSettings))
	mux.HandleFunc("GET /api/instances", s.auth(s.instancesList))
	mux.HandleFunc("POST /api/instances/{instanceID}/launch", s.auth(s.launch))
	mux.HandleFunc("GET /api/instances/{instanceID}/sessions", s.auth(s.sessions))
	mux.HandleFunc("GET /api/instances/{instanceID}/panes/{paneID}/text", s.auth(s.paneText))
	mux.HandleFunc("GET /api/instances/{instanceID}/panes/{paneID}/snapshot", s.auth(s.paneSnapshot))
	mux.HandleFunc("POST /api/instances/{instanceID}/panes/{paneID}/attachments", s.auth(s.uploadAttachments))
	mux.HandleFunc("POST /api/instances/{instanceID}/panes/{paneID}/send", s.auth(s.sendText))
	mux.HandleFunc("DELETE /api/instances/{instanceID}/panes/{paneID}", s.auth(s.deletePane))
	mux.HandleFunc("POST /api/instances/{instanceID}/spawn", s.auth(s.spawn))
	return s.logRequests(mux)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	cfg := s.configSnapshot()
	network := netinfo.Inspect(cfg.Listen)
	writeOK(w, http.StatusOK, map[string]any{
		"service":     "easycodex-agent",
		"time":        time.Now().Format(time.RFC3339Nano),
		"lanEnabled":  network.LANEnabled,
		"machineName": effectiveMachineName(cfg),
	})
}

func (s *Server) instancesList(w http.ResponseWriter, r *http.Request) {
	writeOK(w, http.StatusOK, map[string]any{"instances": s.instanceResponses()})
}

func (s *Server) pairing(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
		writeError(w, http.StatusForbidden, errors.New("pairing is only available from localhost"))
		return
	}
	cfg := s.configSnapshot()
	writeOK(w, http.StatusOK, pairingResponse{
		Service:     "easycodex-agent",
		Network:     netinfo.Inspect(cfg.Listen),
		Token:       cfg.Token,
		Instances:   s.instanceResponses(),
		Defaults:    s.mobileDefaultsResponse(),
		GeneratedAt: time.Now().Format(time.RFC3339Nano),
	})
}

func (s *Server) pairingPage(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
		writeError(w, http.StatusForbidden, errors.New("pairing page is only available from localhost"))
		return
	}
	cfg := s.configSnapshot()
	lang := normalizeUILang(cfg.UILanguage)
	network := netinfo.Inspect(cfg.Listen)
	baseURLs := append([]string(nil), network.LANURLs...)
	if cfg.PublicBaseURL != "" {
		baseURLs = append(baseURLs, cfg.PublicBaseURL)
	}
	if len(baseURLs) == 0 {
		baseURLs = append(baseURLs, network.LocalURL)
	} else {
		baseURLs = append(baseURLs, network.LocalURL)
	}
	s.writePairingConsole(w, lang, baseURLs)
}

func (s *Server) pairingQR(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
		writeError(w, http.StatusForbidden, errors.New("pairing QR is only available from localhost"))
		return
	}
	data := r.URL.Query().Get("data")
	if data == "" {
		writeError(w, http.StatusBadRequest, errors.New("data is required"))
		return
	}
	svg, err := qr.SVG(data, 8)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	_, _ = w.Write([]byte(svg))
}
func (s *Server) mobilePair(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("code") != s.mobilePairCode() {
		writeError(w, http.StatusForbidden, errors.New("invalid pairing code"))
		return
	}
	baseURL := r.URL.Query().Get("baseUrl")
	if baseURL == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		baseURL = scheme + "://" + r.Host
	}
	cfg := s.configSnapshot()
	writeOK(w, http.StatusOK, map[string]any{
		"baseUrl":  baseURL,
		"token":    cfg.Token,
		"defaults": s.mobileDefaultsResponse(),
	})
}

func (s *Server) mobilePairCode() string {
	cfg := s.configSnapshot()
	sum := sha256.Sum256([]byte(cfg.Token + "|" + cfg.Listen + "|" + cfg.PublicBaseURL))
	return hex.EncodeToString(sum[:])[:12]
}

func (s *Server) appConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.configSnapshot()
	writeOK(w, http.StatusOK, appConfigResponse{
		Instances: s.instanceResponses(),
		Defaults:  s.mobileDefaultsResponse(),
		Machine:   effectiveMachineName(cfg),
	})
}

func (s *Server) codexSessions(w http.ResponseWriter, r *http.Request) {
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 20, 100)
	sessions, err := recentCodexSessions(limit)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeOK(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (s *Server) mobileDefaultsResponse() mobileDefaultsResponse {
	cfg := s.configSnapshot()
	command := append([]string(nil), cfg.MobileDefaults.Command...)
	return mobileDefaultsResponse{
		InstanceID: cfg.MobileDefaults.InstanceID,
		CWD:        cfg.MobileDefaults.CWD,
		Command:    command,
	}
}

func (s *Server) instanceResponses() []instanceResponse {
	cfg := s.configSnapshot()
	items := make([]instanceResponse, 0, len(cfg.Instances))
	for _, instance := range cfg.Instances {
		items = append(items, instanceResponse{
			ID:    instance.ID,
			Name:  instance.Name,
			Class: instance.Class,
		})
	}
	return items
}

func (s *Server) launch(w http.ResponseWriter, r *http.Request) {
	instance, ok := s.instance(w, r)
	if !ok {
		return
	}
	pid, err := s.wezterm.Launch(r.Context(), instance.Class)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeOK(w, http.StatusOK, map[string]any{
		"instance": instance.ID,
		"pid":      pid,
	})
}

func (s *Server) sessions(w http.ResponseWriter, r *http.Request) {
	instance, ok := s.instance(w, r)
	if !ok {
		return
	}
	data, err := s.wezterm.List(r.Context(), instance.Class)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	tree, err := normalizeSessions(instance.ID, data)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("invalid wezterm list json: %w", err))
		return
	}
	s.attachPaneInputs(instance.ID, &tree)
	writeOK(w, http.StatusOK, tree)
}

func (s *Server) paneText(w http.ResponseWriter, r *http.Request) {
	instance, ok := s.instance(w, r)
	if !ok {
		return
	}
	paneID := r.PathValue("paneID")
	if !validID(paneID) {
		writeError(w, http.StatusBadRequest, errors.New("invalid pane id"))
		return
	}

	query, ok := parseTextQuery(w, r)
	if !ok {
		return
	}

	text, err := s.wezterm.GetText(r.Context(), instance.Class, paneID, query.Lines, query.Escapes)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeOK(w, http.StatusOK, map[string]any{
		"instance": instance.ID,
		"paneId":   paneID,
		"text":     text,
	})
}

func (s *Server) paneSnapshot(w http.ResponseWriter, r *http.Request) {
	instance, ok := s.instance(w, r)
	if !ok {
		return
	}
	paneID := r.PathValue("paneID")
	if !validID(paneID) {
		writeError(w, http.StatusBadRequest, errors.New("invalid pane id"))
		return
	}
	query, ok := parseTextQuery(w, r)
	if !ok {
		return
	}

	text, err := s.wezterm.GetText(r.Context(), instance.Class, paneID, query.Lines, query.Escapes)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	hash := hashText(text)
	since := r.URL.Query().Get("since")
	changed := since == "" || since != hash
	data := map[string]any{
		"instance":   instance.ID,
		"paneId":     paneID,
		"hash":       hash,
		"changed":    changed,
		"lineCount":  countLines(text),
		"capturedAt": time.Now().Format(time.RFC3339Nano),
	}
	if changed {
		data["text"] = text
	}
	writeOK(w, http.StatusOK, data)
}

func (s *Server) uploadAttachments(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.instance(w, r); !ok {
		return
	}
	paneID := r.PathValue("paneID")
	if !validID(paneID) {
		writeError(w, http.StatusBadRequest, errors.New("invalid pane id"))
		return
	}
	cfg := s.configSnapshot()
	root, err := filepath.Abs(cfg.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAttachmentTotalBytes+1)
	if err := r.ParseMultipartForm(maxAttachmentTotalBytes); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid attachments: %w", err))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		files = r.MultipartForm.File["file"]
	}
	if len(files) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("no attachment files"))
		return
	}

	baseDir := filepath.Join(root, ".attachments", time.Now().Format("20060102"), sanitizePathSegment(paneID))
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	var total int64
	uploads := make([]attachmentUpload, 0, len(files))
	for _, header := range files {
		if header.Size > maxAttachmentFileBytes {
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Errorf("attachment %q exceeds 25MB", header.Filename))
			return
		}
		total += header.Size
		if total > maxAttachmentTotalBytes {
			writeError(w, http.StatusRequestEntityTooLarge, errors.New("attachments exceed 100MB"))
			return
		}
		src, err := header.Open()
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		name := uniqueAttachmentName(header.Filename)
		dstPath := filepath.Join(baseDir, name)
		if !isWithinDir(baseDir, dstPath) {
			_ = src.Close()
			writeError(w, http.StatusBadRequest, errors.New("invalid attachment path"))
			return
		}
		dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			_ = src.Close()
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		written, copyErr := io.Copy(dst, io.LimitReader(src, maxAttachmentFileBytes+1))
		closeErr := dst.Close()
		_ = src.Close()
		if copyErr != nil {
			writeError(w, http.StatusInternalServerError, copyErr)
			return
		}
		if closeErr != nil {
			writeError(w, http.StatusInternalServerError, closeErr)
			return
		}
		if written > maxAttachmentFileBytes {
			_ = os.Remove(dstPath)
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Errorf("attachment %q exceeds 25MB", header.Filename))
			return
		}
		uploads = append(uploads, attachmentUpload{
			OriginalName: header.Filename,
			FileName:     name,
			Path:         dstPath,
			Size:         written,
			MIME:         header.Header.Get("Content-Type"),
		})
	}
	writeOK(w, http.StatusOK, map[string]any{"attachments": uploads})
}

func (s *Server) sendText(w http.ResponseWriter, r *http.Request) {
	instance, ok := s.instance(w, r)
	if !ok {
		return
	}
	paneID := r.PathValue("paneID")
	if !validID(paneID) {
		writeError(w, http.StatusBadRequest, errors.New("invalid pane id"))
		return
	}

	var body sendTextRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	text, err := decodeSendText(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if text == "" && !body.Enter {
		writeError(w, http.StatusBadRequest, errors.New("text is required"))
		return
	}

	if body.Enter {
		text = strings.TrimRight(text, "\r\n")
	}
	if text != "" {
		if err := s.wezterm.SendText(r.Context(), instance.Class, paneID, text, body.NoPaste); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		if shouldRecordPaneInput(text, body.RecordInput) {
			s.recordPaneInput(instance.ID, paneID, text)
		}
	}
	if body.Enter {
		if text != "" {
			time.Sleep(sendEnterDelay(body.EnterDelayMillis))
		}
		if err := s.wezterm.SendText(r.Context(), instance.Class, paneID, "\r", true); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	writeOK(w, http.StatusOK, map[string]any{"sent": true})
}

func (s *Server) deletePane(w http.ResponseWriter, r *http.Request) {
	instance, ok := s.instance(w, r)
	if !ok {
		return
	}
	paneID := r.PathValue("paneID")
	if !validID(paneID) {
		writeError(w, http.StatusBadRequest, errors.New("invalid pane id"))
		return
	}
	if err := s.wezterm.KillPane(r.Context(), instance.Class, paneID); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeOK(w, http.StatusOK, map[string]any{
		"instance": instance.ID,
		"paneId":   paneID,
		"deleted":  true,
	})
}

func (s *Server) spawn(w http.ResponseWriter, r *http.Request) {
	instance, ok := s.instance(w, r)
	if !ok {
		return
	}

	var body struct {
		PaneID    string   `json:"paneId"`
		CWD       string   `json:"cwd"`
		NewWindow bool     `json:"newWindow"`
		Command   []string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	targetPaneID := body.PaneID
	if targetPaneID == "" && !body.NewWindow {
		var err error
		targetPaneID, err = s.activePaneID(r.Context(), instance)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	paneID, err := s.wezterm.Spawn(r.Context(), instance.Class, targetPaneID, body.CWD, body.NewWindow, body.Command)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeOK(w, http.StatusOK, map[string]any{
		"paneId": paneID,
	})
}

func (s *Server) activePaneID(ctx context.Context, instance config.Instance) (string, error) {
	data, err := s.wezterm.List(ctx, instance.Class)
	if err != nil {
		return "", err
	}
	tree, err := normalizeSessions(instance.ID, data)
	if err != nil {
		return "", err
	}
	for _, pane := range tree.Panes {
		if pane.IsActive {
			return pane.PaneID, nil
		}
	}
	if len(tree.Panes) > 0 {
		return tree.Panes[0].PaneID, nil
	}
	return "", errors.New("no panes available for spawn")
}

func (s *Server) recordPaneInput(instanceID, paneID, text string) {
	summary := summarizePaneInput(text, 20)
	if summary == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paneInputs[paneInputKey(instanceID, paneID)] = paneInput{Text: summary, UpdatedAt: time.Now()}
}

func (s *Server) attachPaneInputs(instanceID string, tree *sessionTree) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range tree.Panes {
		input, ok := s.paneInputs[paneInputKey(instanceID, tree.Panes[i].PaneID)]
		if ok {
			tree.Panes[i].LastInput = input.Text
			tree.Panes[i].LastInputAt = input.UpdatedAt.Format(time.RFC3339Nano)
		}
	}
	for wIndex := range tree.Windows {
		for tIndex := range tree.Windows[wIndex].Tabs {
			for pIndex := range tree.Windows[wIndex].Tabs[tIndex].Panes {
				pane := &tree.Windows[wIndex].Tabs[tIndex].Panes[pIndex]
				input, ok := s.paneInputs[paneInputKey(instanceID, pane.PaneID)]
				if ok {
					pane.LastInput = input.Text
					pane.LastInputAt = input.UpdatedAt.Format(time.RFC3339Nano)
				}
			}
		}
	}
}

func paneInputKey(instanceID, paneID string) string {
	return instanceID + "\x00" + paneID
}

func shouldRecordPaneInput(text string, explicit *bool) bool {
	if explicit != nil {
		return *explicit && summarizePaneInput(text, 20) != ""
	}
	return summarizePaneInput(text, 20) != "" && !containsTerminalControl(text)
}

func containsTerminalControl(text string) bool {
	for _, r := range text {
		if r < 32 || r == 127 {
			return true
		}
	}
	return false
}

func uniqueAttachmentName(original string) string {
	clean := sanitizeFileName(original)
	ext := filepath.Ext(clean)
	stem := strings.TrimSuffix(clean, ext)
	if stem == "" {
		stem = "attachment"
	}
	return stem + "-" + randomHex(4) + ext
}

func sanitizeFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		switch r {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
			return '_'
		}
		if r < 32 || r == 127 {
			return '_'
		}
		return r
	}, name)
	name = strings.Trim(name, " .")
	if name == "" || name == "." || name == ".." {
		return "attachment"
	}
	return name
}

func sanitizePathSegment(value string) string {
	value = sanitizeFileName(value)
	if value == "attachment" {
		return "pane"
	}
	return value
}

func randomHex(bytesLen int) string {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}

func isWithinDir(baseDir, path string) bool {
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(baseAbs, pathAbs)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func recentCodexSessions(limit int) ([]codexSessionItem, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(home, ".codex", "sessions")
	var paths []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry == nil || entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".jsonl") {
			return nil
		}
		paths = append(paths, path)
		return nil
	}); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []codexSessionItem{}, nil
		}
		return nil, err
	}
	sort.Slice(paths, func(i, j int) bool {
		left, leftErr := os.Stat(paths[i])
		right, rightErr := os.Stat(paths[j])
		if leftErr != nil || rightErr != nil {
			return paths[i] > paths[j]
		}
		return left.ModTime().After(right.ModTime())
	})
	if limit > len(paths) {
		limit = len(paths)
	}
	items := make([]codexSessionItem, 0, limit)
	for _, path := range paths {
		item, ok := readCodexSessionItem(path)
		if !ok {
			continue
		}
		items = append(items, item)
		if len(items) >= limit {
			break
		}
	}
	return items, nil
}

func readCodexSessionItem(path string) (codexSessionItem, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return codexSessionItem{}, false
	}
	lines := strings.Split(string(data), "\n")
	item := codexSessionItem{Path: path}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		if record["type"] == "session_meta" {
			if payload, ok := record["payload"].(map[string]any); ok {
				item.ID = stringField(payload, "id")
				item.CWD = stringField(payload, "cwd")
				item.Timestamp = stringField(payload, "timestamp")
			}
			continue
		}
		if item.Summary == "" {
			item.Summary = summarizeCodexRecord(record)
		}
		if item.ID != "" && item.Summary != "" {
			break
		}
	}
	if item.ID == "" {
		item.ID = codexSessionIDFromPath(path)
	}
	if item.Timestamp == "" {
		if info, err := os.Stat(path); err == nil {
			item.Timestamp = info.ModTime().Format(time.RFC3339)
		}
	}
	if item.Summary == "" {
		item.Summary = filepath.Base(path)
	}
	item.Summary = summarizePaneInput(item.Summary, 60)
	return item, item.ID != ""
}

func summarizeCodexRecord(record map[string]any) string {
	payload, ok := record["payload"].(map[string]any)
	if !ok {
		return ""
	}
	if role := stringField(payload, "role"); role != "" && role != "user" {
		return ""
	}
	if text := codexUserSummaryCandidate(stringField(payload, "message")); text != "" {
		return text
	}
	if text := codexUserSummaryCandidate(stringField(payload, "text")); text != "" {
		return text
	}
	content, ok := payload["content"].([]any)
	if !ok {
		return ""
	}
	for _, part := range content {
		if value, ok := part.(map[string]any); ok {
			if text := codexUserSummaryCandidate(stringField(value, "text")); text != "" {
				return text
			}
		}
	}
	return ""
}

func codexUserSummaryCandidate(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	for _, prefix := range []string{
		"<environment_context>",
		"<permissions instructions>",
		"<collaboration_mode>",
		"<skills_instructions>",
	} {
		if strings.HasPrefix(text, prefix) {
			return ""
		}
	}
	return text
}

func stringField(values map[string]any, key string) string {
	if value, ok := values[key].(string); ok {
		return value
	}
	return ""
}

func codexSessionIDFromPath(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if len(name) >= 36 {
		candidate := name[len(name)-36:]
		if strings.Count(candidate, "-") == 4 {
			return candidate
		}
	}
	return ""
}

func summarizePaneInput(text string, limit int) string {
	text = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, text))
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if limit > 0 && len(runes) > limit {
		return string(runes[:limit]) + "..."
	}
	return text
}

func (s *Server) instance(w http.ResponseWriter, r *http.Request) (config.Instance, bool) {
	id := r.PathValue("instanceID")
	s.mu.RLock()
	instance, ok := s.instances[id]
	s.mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("unknown instance %q", id))
		return config.Instance{}, false
	}
	return instance, true
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := s.configSnapshot()
		if cfg.Token == "" {
			next(w, r)
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" {
			token = r.Header.Get("X-EasyCodex-Token")
		}
		if token != cfg.Token {
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}
		s.recordClient(r)
		next(w, r)
	}
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.logger.Info("request", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) localOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLocalRequest(r) {
			writeError(w, http.StatusForbidden, errors.New("this endpoint is only available from localhost"))
			return
		}
		next(w, r)
	}
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	cfg := s.configSnapshot()
	writeOK(w, http.StatusOK, settingsResponse{
		Config:     cfg,
		ConfigPath: s.configPath,
		Network:    netinfo.Inspect(cfg.Listen),
		Version:    AppVersion,
	})
}

func (s *Server) SetRestartFunc(restart func()) {
	s.mu.Lock()
	s.restart = restart
	s.mu.Unlock()
}

func (s *Server) restartAgent(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	restart := s.restart
	s.mu.RUnlock()
	if restart == nil {
		writeError(w, http.StatusNotImplemented, errors.New("restart is not available"))
		return
	}
	writeOK(w, http.StatusAccepted, map[string]bool{"restarting": true})
	go restart()
}

func (s *Server) connections(w http.ResponseWriter, r *http.Request) {
	writeOK(w, http.StatusOK, map[string]any{"connections": s.connectionSnapshot()})
}

func (s *Server) networkTests(w http.ResponseWriter, r *http.Request) {
	targets := s.networkTestTargets()
	results := make([]networkTestResult, len(targets))
	var wg sync.WaitGroup
	for i, target := range targets {
		wg.Add(1)
		go func(index int, target networkTestTarget) {
			defer wg.Done()
			results[index] = testNetworkTarget(r.Context(), target)
		}(i, target)
	}
	wg.Wait()
	writeOK(w, http.StatusOK, map[string]any{"targets": targets, "results": results})
}

func (s *Server) networkTestTargets() []networkTestTarget {
	cfg := s.configSnapshot()
	network := netinfo.Inspect(cfg.Listen)
	targets := []networkTestTarget{{Label: "Local", URL: network.LocalURL}}
	for _, lanURL := range network.LANURLs {
		targets = append(targets, networkTestTarget{Label: "LAN", URL: lanURL})
	}
	if cfg.PublicBaseURL != "" {
		targets = append(targets, networkTestTarget{Label: "Public", URL: cfg.PublicBaseURL})
	}
	seen := map[string]struct{}{}
	deduped := make([]networkTestTarget, 0, len(targets))
	for _, target := range targets {
		target.URL = strings.TrimRight(strings.TrimSpace(target.URL), "/")
		if target.URL == "" {
			continue
		}
		key := strings.ToLower(target.URL)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, target)
	}
	return deduped
}

func testNetworkTarget(parent context.Context, target networkTestTarget) networkTestResult {
	start := time.Now()
	result := networkTestResult{
		Label:    target.Label,
		URL:      target.URL,
		TestedAt: start.Format(time.RFC3339),
	}
	ctx, cancel := context.WithTimeout(parent, 2500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.URL+"/api/health", nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "EasyCodex-network-test")
	res, err := updateHTTPClient.Do(req)
	result.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer res.Body.Close()
	result.Status = res.StatusCode
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Service    string `json:"service"`
			LANEnabled bool   `json:"lanEnabled"`
		} `json:"data"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		result.Error = err.Error()
		return result
	}
	result.OK = res.StatusCode >= 200 && res.StatusCode < 300 && payload.OK && payload.Data.Service == "easycodex-agent"
	result.Service = payload.Data.Service
	result.LANEnabled = &payload.Data.LANEnabled
	if !result.OK && payload.Error != "" {
		result.Error = payload.Error
	}
	return result
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	var next config.Config
	if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if next.Token == "" {
		token, err := config.GenerateToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		next.Token = token
	}
	config.Normalize(&next)
	if err := config.Validate(next); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	current := s.configSnapshot()
	if err := config.Save(s.configPath, next); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.setConfig(next)
	restartFields := changedRestartFields(current, next)
	writeOK(w, http.StatusOK, settingsResponse{
		Config:          next,
		ConfigPath:      s.configPath,
		Network:         netinfo.Inspect(next.Listen),
		Version:         AppVersion,
		RestartRequired: len(restartFields) > 0,
		RestartFields:   restartFields,
	})
}

func (s *Server) configSnapshot() config.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneConfig(s.cfg)
}

func (s *Server) setConfig(cfg config.Config) {
	instances := make(map[string]config.Instance, len(cfg.Instances))
	for _, instance := range cfg.Instances {
		instances[instance.ID] = instance
	}
	s.mu.Lock()
	s.cfg = cloneConfig(cfg)
	s.instances = instances
	s.mu.Unlock()
}

func (s *Server) recordClient(r *http.Request) {
	now := time.Now()
	remote := remoteHost(r.RemoteAddr)
	kind := clientKind(r)
	name := strings.TrimSpace(r.Header.Get("X-EasyCodex-Client-Name"))
	if name == "" {
		name = kind
	}
	id := strings.TrimSpace(r.Header.Get("X-EasyCodex-Client-ID"))
	if id == "" {
		sum := sha256.Sum256([]byte(remote + "|" + r.UserAgent() + "|" + kind))
		id = "auto:" + hex.EncodeToString(sum[:])[:16]
	}
	if len(id) > 96 {
		id = id[:96]
	}

	s.mu.Lock()
	item := s.clients[id]
	if item.Requests == 0 {
		item.ID = id
		item.FirstSeen = formatConnectionTime(now)
		item.firstSeenTime = now
	}
	item.Kind = kind
	item.Name = name
	item.RemoteAddr = remote
	item.UserAgent = r.UserAgent()
	item.LastMethod = r.Method
	item.LastPath = r.URL.Path
	item.LastSeen = formatConnectionTime(now)
	item.lastSeenTime = now
	item.Requests++
	s.clients[id] = item
	s.pruneClientsLocked(now)
	s.mu.Unlock()
}

func (s *Server) connectionSnapshot() []clientConnection {
	s.mu.RLock()
	items := make([]clientConnection, 0, len(s.clients))
	for _, item := range s.clients {
		items = append(items, item)
	}
	s.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool {
		return items[i].lastSeenTime.After(items[j].lastSeenTime)
	})
	return items
}

func (s *Server) pruneClientsLocked(now time.Time) {
	const maxClients = 200
	const maxAge = 72 * time.Hour
	for id, item := range s.clients {
		if now.Sub(item.lastSeenTime) > maxAge {
			delete(s.clients, id)
		}
	}
	if len(s.clients) <= maxClients {
		return
	}
	items := make([]clientConnection, 0, len(s.clients))
	for _, item := range s.clients {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].lastSeenTime.Before(items[j].lastSeenTime)
	})
	for _, item := range items[:len(s.clients)-maxClients] {
		delete(s.clients, item.ID)
	}
}

func cloneConfig(cfg config.Config) config.Config {
	cfg.AutoLaunch = append([]string(nil), cfg.AutoLaunch...)
	cfg.Instances = append([]config.Instance(nil), cfg.Instances...)
	cfg.MobileDefaults.Command = append([]string(nil), cfg.MobileDefaults.Command...)
	return cfg
}

func changedRestartFields(before, after config.Config) []string {
	fields := []string{}
	if before.Listen != after.Listen {
		fields = append(fields, "listen")
	}
	if before.Root != after.Root {
		fields = append(fields, "root")
	}
	if before.CommandTimeoutSeconds != after.CommandTimeoutSeconds {
		fields = append(fields, "commandTimeoutSeconds")
	}
	if before.PublicBaseURL != after.PublicBaseURL {
		fields = append(fields, "publicBaseUrl")
	}
	if before.RegenerateTokenOnStart != after.RegenerateTokenOnStart {
		fields = append(fields, "regenerateTokenOnStart")
	}
	if strings.Join(before.AutoLaunch, "\x00") != strings.Join(after.AutoLaunch, "\x00") {
		fields = append(fields, "autoLaunch")
	}
	if before.CloseLaunchedGUIOnExit != after.CloseLaunchedGUIOnExit {
		fields = append(fields, "closeLaunchedGuiOnExit")
	}
	return fields
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeOK(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, apiResponse{OK: true, Data: data})
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, apiResponse{OK: false, Error: err.Error()})
}

func validID(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func parseTextQuery(w http.ResponseWriter, r *http.Request) (textQuery, bool) {
	query := textQuery{Lines: 200, Escapes: parseBool(r.URL.Query().Get("escapes"))}
	if raw := r.URL.Query().Get("lines"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 5000 {
			writeError(w, http.StatusBadRequest, errors.New("lines must be between 1 and 5000"))
			return textQuery{}, false
		}
		query.Lines = value
	}
	return query, true
}

func parsePositiveInt(raw string, fallback, max int) int {
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return fallback
	}
	if max > 0 && value > max {
		return max
	}
	return value
}

func parseBool(value string) bool {
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func isLocalRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func remoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

func clientKind(r *http.Request) string {
	raw := strings.ToLower(strings.TrimSpace(r.Header.Get("X-EasyCodex-Client-Kind")))
	switch raw {
	case "android", "android app", "apk":
		return "Android App"
	case "browser", "web", "browser terminal":
		return "Browser"
	}
	ua := strings.ToLower(r.UserAgent())
	switch {
	case strings.Contains(ua, "easycodex-android"):
		return "Android App"
	case strings.Contains(ua, "mozilla"):
		return "Browser"
	default:
		return "API Client"
	}
}

func formatConnectionTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

func decodeSendText(body sendTextRequest) (string, error) {
	if body.Text != "" && body.TextBase64 != "" {
		return "", errors.New("text and textBase64 cannot both be set")
	}
	if body.TextBase64 == "" {
		return body.Text, nil
	}
	data, err := base64.StdEncoding.DecodeString(body.TextBase64)
	if err != nil {
		return "", fmt.Errorf("invalid textBase64: %w", err)
	}
	return string(data), nil
}

func sendEnterDelay(value int) time.Duration {
	if value < 0 {
		return 0
	}
	if value == 0 {
		return 100 * time.Millisecond
	}
	if value > 2000 {
		value = 2000
	}
	return time.Duration(value) * time.Millisecond
}

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func countLines(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

func normalizeSessions(instanceID string, data json.RawMessage) (sessionTree, error) {
	var rawPanes []weztermPane
	if err := json.Unmarshal(data, &rawPanes); err != nil {
		return sessionTree{}, err
	}

	tree := sessionTree{
		Instance: instanceID,
		Windows:  []windowSession{},
		Panes:    make([]paneSession, 0, len(rawPanes)),
	}

	windowIndex := map[int]int{}
	tabIndex := map[int]map[int]int{}
	for _, raw := range rawPanes {
		pane := paneFromWezTerm(raw)
		if pane.IsWorking {
			tree.WorkingCount++
		}
		tree.Panes = append(tree.Panes, pane)

		wIndex, ok := windowIndex[raw.WindowID]
		if !ok {
			windowIndex[raw.WindowID] = len(tree.Windows)
			tabIndex[raw.WindowID] = map[int]int{}
			tree.Windows = append(tree.Windows, windowSession{
				WindowID:  raw.WindowID,
				Title:     raw.WindowTitle,
				Workspace: raw.Workspace,
				IsActive:  raw.IsActive,
				Tabs:      []tabSession{},
			})
			wIndex = len(tree.Windows) - 1
		}
		if raw.IsActive {
			tree.Windows[wIndex].IsActive = true
		}
		if tree.Windows[wIndex].Title == "" {
			tree.Windows[wIndex].Title = raw.WindowTitle
		}

		tIndex, ok := tabIndex[raw.WindowID][raw.TabID]
		if !ok {
			tabIndex[raw.WindowID][raw.TabID] = len(tree.Windows[wIndex].Tabs)
			tree.Windows[wIndex].Tabs = append(tree.Windows[wIndex].Tabs, tabSession{
				TabID:    raw.TabID,
				Title:    raw.TabTitle,
				IsActive: raw.IsActive,
				IsZoomed: raw.IsZoomed,
				Panes:    []paneSession{},
			})
			tIndex = len(tree.Windows[wIndex].Tabs) - 1
		}
		tab := &tree.Windows[wIndex].Tabs[tIndex]
		if raw.IsActive {
			tab.IsActive = true
		}
		if raw.IsZoomed {
			tab.IsZoomed = true
		}
		if tab.Title == "" {
			tab.Title = raw.TabTitle
		}
		tab.Panes = append(tab.Panes, pane)
	}

	sortSessionTree(&tree)
	return tree, nil
}

func paneFromWezTerm(raw weztermPane) paneSession {
	return paneSession{
		PaneID:           strconv.Itoa(raw.PaneID),
		WindowID:         raw.WindowID,
		TabID:            raw.TabID,
		Title:            raw.Title,
		CWD:              raw.CWD,
		Workspace:        raw.Workspace,
		IsActive:         raw.IsActive,
		IsWorking:        isWorkingPane(raw),
		IsZoomed:         raw.IsZoomed,
		CursorX:          raw.CursorX,
		CursorY:          raw.CursorY,
		CursorShape:      raw.CursorShape,
		CursorVisibility: raw.CursorVisibility,
		LeftCol:          raw.LeftCol,
		TopRow:           raw.TopRow,
		TTYName:          raw.TTYName,
		Size:             raw.Size,
	}
}

func isWorkingPane(raw weztermPane) bool {
	title := strings.ToLower(raw.Title + " " + raw.TabTitle)
	for _, marker := range []string{"working", "thinking", "running"} {
		if strings.Contains(title, marker) {
			return true
		}
	}
	original := raw.Title + " " + raw.TabTitle
	for _, frame := range codexSpinnerFrames {
		if strings.Contains(original, frame) {
			return true
		}
	}
	return false
}

var codexSpinnerFrames = []string{
	"\u280b", "\u2819", "\u2839", "\u2838", "\u283c", "\u2834", "\u2826", "\u2827", "\u2807", "\u280f",
	"\u25d0", "\u25d3", "\u25d1", "\u25d2",
}

func sortSessionTree(tree *sessionTree) {
	sort.Slice(tree.Panes, func(i, j int) bool {
		return paneLess(tree.Panes[i], tree.Panes[j])
	})
	sort.Slice(tree.Windows, func(i, j int) bool {
		return tree.Windows[i].WindowID < tree.Windows[j].WindowID
	})
	for wIndex := range tree.Windows {
		window := &tree.Windows[wIndex]
		sort.Slice(window.Tabs, func(i, j int) bool {
			return window.Tabs[i].TabID < window.Tabs[j].TabID
		})
		for tIndex := range window.Tabs {
			tab := &window.Tabs[tIndex]
			sort.Slice(tab.Panes, func(i, j int) bool {
				return paneLess(tab.Panes[i], tab.Panes[j])
			})
		}
	}
}

func paneLess(left, right paneSession) bool {
	if left.WindowID != right.WindowID {
		return left.WindowID < right.WindowID
	}
	if left.TabID != right.TabID {
		return left.TabID < right.TabID
	}
	return left.PaneID < right.PaneID
}
