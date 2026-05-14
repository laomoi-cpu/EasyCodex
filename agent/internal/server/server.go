package server

import (
	"bytes"
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
	"net/url"
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
	SetTabTitle(ctx context.Context, class, paneID, title string) error
	KillPane(ctx context.Context, class, paneID string) error
	Spawn(ctx context.Context, class, paneID, cwd string, newWindow bool, command []string) (string, error)
}

type Server struct {
	mu              sync.RWMutex
	cfg             config.Config
	configPath      string
	wezterm         WezTerm
	instances       map[string]config.Instance
	clients         map[string]clientConnection
	paneInputs      map[string]paneInput
	syncedTabTitles map[string]string
	restart         func()
	updateJob       updateJobStatus
	codexCache      []codexSessionItem
	codexCacheAt    time.Time
	codexCacheLimit int
	logger          *slog.Logger
}

type paneInput struct {
	Text           string
	CodexSessionID string
	UpdatedAt      time.Time
}

type paneInputState struct {
	Version int                  `json:"version"`
	Inputs  []paneInputStateItem `json:"inputs"`
}

type paneInputStateItem struct {
	InstanceID     string `json:"instanceId"`
	PaneID         string `json:"paneId"`
	Text           string `json:"text,omitempty"`
	CodexSessionID string `json:"codexSessionId,omitempty"`
	UpdatedAt      string `json:"updatedAt,omitempty"`
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
	Instances          []instanceResponse     `json:"instances"`
	Defaults           mobileDefaultsResponse `json:"defaults"`
	Machine            string                 `json:"machineName"`
	AutoScrollTerminal bool                   `json:"autoScrollTerminal"`
	RetentionLines     int                    `json:"terminalRetentionLines"`
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

type codexSessionTitleRequest struct {
	Title string `json:"title"`
}

type codexSessionOpenRequest struct {
	InstanceID string `json:"instanceId"`
	NewWindow  bool   `json:"newWindow"`
}

type paneCodexSessionRequest struct {
	CodexSessionID string `json:"codexSessionId"`
}

type attachmentUpload struct {
	OriginalName string `json:"originalName"`
	FileName     string `json:"fileName"`
	Path         string `json:"path"`
	Size         int64  `json:"size"`
	MIME         string `json:"mime,omitempty"`
}

type codexSessionItem struct {
	ID           string `json:"id"`
	CWD          string `json:"cwd,omitempty"`
	Timestamp    string `json:"timestamp,omitempty"`
	Summary      string `json:"summary,omitempty"`
	CustomTitle  string `json:"customTitle,omitempty"`
	DisplayTitle string `json:"displayTitle,omitempty"`
	Path         string `json:"path,omitempty"`
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
	ConfirmCount int             `json:"confirmCount"`
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
	DisplayTitle     string          `json:"displayTitle,omitempty"`
	CustomTitle      string          `json:"customTitle,omitempty"`
	CodexSessionID   string          `json:"codexSessionId,omitempty"`
	CWD              string          `json:"cwd"`
	LastInput        string          `json:"lastInput,omitempty"`
	LastInputAt      string          `json:"lastInputAt,omitempty"`
	Workspace        string          `json:"workspace"`
	IsActive         bool            `json:"isActive"`
	IsWorking        bool            `json:"isWorking"`
	IsConfirm        bool            `json:"isConfirm"`
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
	paneInputs, err := loadPaneInputs(cfg.Root)
	if err != nil {
		logger.Warn("failed to load pane input state", "err", err)
		paneInputs = map[string]paneInput{}
	}
	return &Server{cfg: cfg, configPath: configPath, wezterm: wezterm, instances: instances, clients: map[string]clientConnection{}, paneInputs: paneInputs, syncedTabTitles: map[string]string{}, logger: logger}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.homePage)
	mux.HandleFunc("GET /status", s.statusPage)
	mux.HandleFunc("GET /settings", s.settingsPage)
	mux.HandleFunc("GET /connections", s.connectionsPage)
	mux.HandleFunc("GET /terminal", s.terminalPage)
	mux.HandleFunc("GET /sessions", s.sessionsPage)
	mux.HandleFunc("GET /assets/easycodex.svg", s.easycodexIcon)
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/pairing", s.pairing)
	mux.HandleFunc("GET /pairing", s.pairingPage)
	mux.HandleFunc("GET /api/pairing/qr.svg", s.pairingQR)
	mux.HandleFunc("GET /api/mobile-pair", s.mobilePair)
	mux.HandleFunc("GET /api/config", s.auth(s.appConfig))
	mux.HandleFunc("GET /api/codex/sessions", s.auth(s.codexSessions))
	mux.HandleFunc("PUT /api/codex/sessions/{sessionID}/title", s.auth(s.saveCodexSessionTitle))
	mux.HandleFunc("POST /api/codex/sessions/{sessionID}/open", s.localOnly(s.openCodexSession))
	mux.HandleFunc("GET /api/settings", s.localOnly(s.settings))
	mux.HandleFunc("GET /api/connections", s.localOnly(s.connections))
	mux.HandleFunc("GET /api/network-tests", s.localOnly(s.networkTests))
	mux.HandleFunc("GET /api/update/check", s.localOrAuth(s.checkUpdate))
	mux.HandleFunc("GET /api/update/status", s.localOrAuth(s.updateStatus))
	mux.HandleFunc("POST /api/update/apply", s.localOrAuth(s.applyUpdate))
	mux.HandleFunc("POST /api/restart", s.localOnly(s.restartAgent))
	mux.HandleFunc("POST /api/settings", s.localOnly(s.saveSettings))
	mux.HandleFunc("GET /api/instances", s.auth(s.instancesList))
	mux.HandleFunc("POST /api/instances/{instanceID}/launch", s.auth(s.launch))
	mux.HandleFunc("GET /api/instances/{instanceID}/sessions", s.auth(s.sessions))
	mux.HandleFunc("GET /api/instances/{instanceID}/panes/{paneID}/text", s.auth(s.paneText))
	mux.HandleFunc("GET /api/instances/{instanceID}/panes/{paneID}/snapshot", s.auth(s.paneSnapshot))
	mux.HandleFunc("PUT /api/instances/{instanceID}/panes/{paneID}/codex-session", s.auth(s.savePaneCodexSession))
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
		baseURL = requestBaseURL(r)
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

func requestBaseURL(r *http.Request) string {
	scheme := forwardedProto(r)
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	host := firstHeaderValue(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	return strings.TrimRight(scheme+"://"+host, "/")
}

func forwardedProto(r *http.Request) string {
	if proto := firstHeaderValue(r.Header.Get("X-Forwarded-Proto")); proto == "http" || proto == "https" {
		return proto
	}
	if strings.EqualFold(firstHeaderValue(r.Header.Get("X-Forwarded-Ssl")), "on") {
		return "https"
	}
	for _, part := range strings.Split(r.Header.Get("Forwarded"), ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || !strings.EqualFold(key, "proto") {
			continue
		}
		proto := strings.ToLower(strings.Trim(strings.TrimSpace(value), `"`))
		if proto == "http" || proto == "https" {
			return proto
		}
	}
	return ""
}

func firstHeaderValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	first, _, _ := strings.Cut(value, ",")
	return strings.TrimSpace(first)
}

func (s *Server) appConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.configSnapshot()
	writeOK(w, http.StatusOK, appConfigResponse{
		Instances:          s.instanceResponses(),
		Defaults:           s.mobileDefaultsResponse(),
		Machine:            effectiveMachineName(cfg),
		AutoScrollTerminal: cfg.AutoScrollTerminal,
		RetentionLines:     cfg.TerminalRetentionLines,
	})
}

func (s *Server) codexSessions(w http.ResponseWriter, r *http.Request) {
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 20, 100)
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	all := parseBool(r.URL.Query().Get("all"))
	var sessions []codexSessionItem
	var err error
	if all || query != "" {
		limit = 0
		if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
			limit = parsePositiveInt(rawLimit, 0, 10000)
		}
		if query != "" {
			limit = 0
		}
		sessions, err = codexSessionsFromHistory(limit, "")
	} else {
		sessions, err = s.recentCodexSessionsCached(limit)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	s.applyCodexSessionTitles(sessions)
	if query != "" {
		filtered := sessions[:0]
		lowerQuery := strings.ToLower(query)
		for _, session := range sessions {
			if codexSessionMatchesQuery(session, session.Path, lowerQuery) {
				filtered = append(filtered, session)
			}
		}
		sessions = filtered
	}
	writeOK(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (s *Server) saveCodexSessionTitle(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if !validCodexSessionID(sessionID) {
		writeError(w, http.StatusBadRequest, errors.New("invalid codex session id"))
		return
	}
	var body codexSessionTitleRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	title := strings.TrimSpace(body.Title)
	if len([]rune(title)) > 80 {
		writeError(w, http.StatusBadRequest, errors.New("title must be 80 characters or fewer"))
		return
	}

	next := s.configSnapshot()
	if next.CodexSessionTitles == nil {
		next.CodexSessionTitles = map[string]string{}
	}
	if title == "" {
		delete(next.CodexSessionTitles, sessionID)
	} else {
		next.CodexSessionTitles[sessionID] = title
	}
	config.Normalize(&next)
	if err := config.Save(s.configPath, next); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.setConfig(next)
	s.syncCodexSessionTitle(r.Context(), sessionID, title)
	writeOK(w, http.StatusOK, map[string]any{
		"sessionId": sessionID,
		"title":     title,
		"titles":    next.CodexSessionTitles,
	})
}

func (s *Server) openCodexSession(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if !validCodexSessionID(sessionID) {
		writeError(w, http.StatusBadRequest, errors.New("invalid codex session id"))
		return
	}
	var body codexSessionOpenRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	session, ok, err := findCodexSession(sessionID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("codex session not found"))
		return
	}
	cfg := s.configSnapshot()
	instance, ok := resolveOpenInstance(cfg, body.InstanceID)
	if !ok {
		writeError(w, http.StatusBadRequest, errors.New("no wezterm instance configured"))
		return
	}
	cwd := session.CWD
	if strings.TrimSpace(cwd) == "" {
		cwd = cfg.MobileDefaults.CWD
	}
	command := []string{"cmd.exe", "/k", "codex", "resume", "--dangerously-bypass-approvals-and-sandbox", sessionID}

	targetPaneID := ""
	newWindow := body.NewWindow
	if !newWindow {
		activePaneID, activeErr := s.activePaneID(r.Context(), instance)
		if activeErr == nil {
			targetPaneID = activePaneID
		} else {
			newWindow = true
		}
	}
	paneID, err := s.wezterm.Spawn(r.Context(), instance.Class, targetPaneID, cwd, newWindow, command)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	s.recordPaneCodexSession(instance.ID, paneID, sessionID)
	if title := s.codexSessionDisplayTitle(session); title != "" {
		s.syncTabTitle(r.Context(), tabTitleSyncTarget{
			InstanceID: instance.ID,
			Class:      instance.Class,
			PaneID:     paneID,
			Title:      title,
		})
	}
	writeOK(w, http.StatusOK, map[string]any{
		"instance":       instance.ID,
		"paneId":         paneID,
		"codexSessionId": sessionID,
	})
}

func (s *Server) ForceSyncWezTermTitles(ctx context.Context) {
	cfg := s.configSnapshot()
	for _, instance := range cfg.Instances {
		data, err := s.wezterm.List(ctx, instance.Class)
		if err != nil {
			s.logger.Debug("skip wezterm title startup sync", "instance", instance.ID, "class", instance.Class, "error", err)
			continue
		}
		tree, err := normalizeSessions(instance.ID, data)
		if err != nil {
			s.logger.Warn("failed to normalize wezterm sessions for title startup sync", "instance", instance.ID, "error", err)
			continue
		}
		s.attachPaneInputs(instance.ID, &tree)
		s.hydratePaneCodexSessions(ctx, instance.ID, instance.Class, &tree)
		s.attachPaneInputs(instance.ID, &tree)
		s.attachCodexSessionTitles(instance.ID, &tree)
		s.syncSessionTreeTitles(ctx, instance, &tree, true)
	}
}

func resolveOpenInstance(cfg config.Config, requested string) (config.Instance, bool) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = strings.TrimSpace(cfg.MobileDefaults.InstanceID)
	}
	if requested != "" {
		for _, instance := range cfg.Instances {
			if instance.ID == requested {
				return instance, true
			}
		}
	}
	if len(cfg.Instances) > 0 {
		return cfg.Instances[0], true
	}
	return config.Instance{}, false
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
	s.hydratePaneCodexSessions(r.Context(), instance.ID, instance.Class, &tree)
	s.attachPaneInputs(instance.ID, &tree)
	s.attachCodexSessionTitles(instance.ID, &tree)
	s.syncSessionTreeTitles(r.Context(), instance, &tree, false)
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

func (s *Server) savePaneCodexSession(w http.ResponseWriter, r *http.Request) {
	instance, ok := s.instance(w, r)
	if !ok {
		return
	}
	paneID := r.PathValue("paneID")
	if !validID(paneID) {
		writeError(w, http.StatusBadRequest, errors.New("invalid pane id"))
		return
	}
	var body paneCodexSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	sessionID := strings.TrimSpace(body.CodexSessionID)
	if !validCodexSessionID(sessionID) {
		writeError(w, http.StatusBadRequest, errors.New("invalid codex session id"))
		return
	}
	s.recordPaneCodexSession(instance.ID, paneID, sessionID)
	writeOK(w, http.StatusOK, map[string]any{
		"instance":       instance.ID,
		"paneId":         paneID,
		"codexSessionId": sessionID,
	})
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
		PaneID         string   `json:"paneId"`
		CWD            string   `json:"cwd"`
		NewWindow      bool     `json:"newWindow"`
		Command        []string `json:"command"`
		CodexSessionID string   `json:"codexSessionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.CodexSessionID != "" && !validCodexSessionID(body.CodexSessionID) {
		writeError(w, http.StatusBadRequest, errors.New("invalid codex session id"))
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
	if body.CodexSessionID != "" {
		s.recordPaneCodexSession(instance.ID, paneID, body.CodexSessionID)
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
	codexSessionID := codexSessionIDFromCommand(text)
	if summary == "" && codexSessionID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	input := s.paneInputs[paneInputKey(instanceID, paneID)]
	if summary != "" {
		input.Text = summary
	}
	if codexSessionID != "" {
		input.CodexSessionID = codexSessionID
	}
	input.UpdatedAt = time.Now()
	s.paneInputs[paneInputKey(instanceID, paneID)] = input
	if err := s.savePaneInputsLocked(); err != nil {
		s.logger.Warn("failed to save pane input state", "err", err)
	}
}

func (s *Server) recordPaneCodexSession(instanceID, paneID, sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	input := s.paneInputs[paneInputKey(instanceID, paneID)]
	input.CodexSessionID = sessionID
	input.UpdatedAt = time.Now()
	s.paneInputs[paneInputKey(instanceID, paneID)] = input
	if err := s.savePaneInputsLocked(); err != nil {
		s.logger.Warn("failed to save pane input state", "err", err)
	}
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

func splitPaneInputKey(key string) (string, string, bool) {
	instanceID, paneID, ok := strings.Cut(key, "\x00")
	return instanceID, paneID, ok
}

func paneInputsStatePath(root string) string {
	return filepath.Join(root, ".state", "pane-inputs.json")
}

func loadPaneInputs(root string) (map[string]paneInput, error) {
	path := paneInputsStatePath(root)
	inputs, err := loadPaneInputsFile(path)
	if err == nil {
		return inputs, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return map[string]paneInput{}, nil
	}
	if backupInputs, backupErr := loadPaneInputsFile(path + ".bak"); backupErr == nil {
		return backupInputs, nil
	}
	return nil, err
}

func loadPaneInputsFile(path string) (map[string]paneInput, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state paneInputState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	inputs := make(map[string]paneInput, len(state.Inputs))
	for _, item := range state.Inputs {
		instanceID := strings.TrimSpace(item.InstanceID)
		paneID := strings.TrimSpace(item.PaneID)
		if instanceID == "" || paneID == "" {
			continue
		}
		input := paneInput{
			Text:           strings.TrimSpace(item.Text),
			CodexSessionID: strings.TrimSpace(item.CodexSessionID),
		}
		if item.UpdatedAt != "" {
			if updatedAt, err := time.Parse(time.RFC3339Nano, item.UpdatedAt); err == nil {
				input.UpdatedAt = updatedAt
			}
		}
		if input.Text == "" && input.CodexSessionID == "" {
			continue
		}
		inputs[paneInputKey(instanceID, paneID)] = input
	}
	return inputs, nil
}

func (s *Server) savePaneInputsLocked() error {
	state := paneInputState{
		Version: 1,
		Inputs:  make([]paneInputStateItem, 0, len(s.paneInputs)),
	}
	keys := make([]string, 0, len(s.paneInputs))
	for key := range s.paneInputs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		input := s.paneInputs[key]
		if input.Text == "" && input.CodexSessionID == "" {
			continue
		}
		instanceID, paneID, ok := splitPaneInputKey(key)
		if !ok || instanceID == "" || paneID == "" {
			continue
		}
		item := paneInputStateItem{
			InstanceID:     instanceID,
			PaneID:         paneID,
			Text:           input.Text,
			CodexSessionID: input.CodexSessionID,
		}
		if !input.UpdatedAt.IsZero() {
			item.UpdatedAt = input.UpdatedAt.Format(time.RFC3339Nano)
		}
		state.Inputs = append(state.Inputs, item)
	}
	return savePaneInputState(paneInputsStatePath(s.cfg.Root), state)
}

func savePaneInputState(path string, state paneInputState) error {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if current, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".bak", current, 0o600); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err == nil {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(tmpName, path)
}

func (s *Server) attachCodexSessionTitles(instanceID string, tree *sessionTree) {
	cfg := s.configSnapshot()
	inputs := s.paneInputsSnapshot(instanceID)
	sessionIDs := make([]string, 0, len(inputs))
	for _, input := range inputs {
		if input.CodexSessionID != "" {
			sessionIDs = append(sessionIDs, input.CodexSessionID)
		}
	}
	titles := s.codexSessionDisplayTitles(sessionIDs)
	enrich := func(pane *paneSession) {
		input := inputs[pane.PaneID]
		sessionID := input.CodexSessionID
		if sessionID == "" {
			return
		}
		pane.CodexSessionID = sessionID
		if title := titles[sessionID]; title != "" {
			if customTitle := cfg.CodexSessionTitles[sessionID]; customTitle != "" {
				pane.CustomTitle = customTitle
			}
			pane.DisplayTitle = title
		}
	}
	for i := range tree.Panes {
		enrich(&tree.Panes[i])
	}
	for wIndex := range tree.Windows {
		for tIndex := range tree.Windows[wIndex].Tabs {
			for pIndex := range tree.Windows[wIndex].Tabs[tIndex].Panes {
				enrich(&tree.Windows[wIndex].Tabs[tIndex].Panes[pIndex])
			}
		}
	}
}

func (s *Server) hydratePaneCodexSessions(ctx context.Context, instanceID, class string, tree *sessionTree) {
	inputs := s.paneInputsSnapshot(instanceID)
	for _, pane := range tree.Panes {
		if inputs[pane.PaneID].CodexSessionID != "" {
			continue
		}
		text, err := s.wezterm.GetText(ctx, class, pane.PaneID, 80, false)
		if err != nil {
			s.logger.Debug("failed to inspect pane text for codex session", "instance", instanceID, "pane", pane.PaneID, "error", err)
			continue
		}
		sessionID := codexSessionIDFromText(text)
		if sessionID == "" {
			continue
		}
		s.recordPaneCodexSession(instanceID, pane.PaneID, sessionID)
		inputs[pane.PaneID] = paneInput{CodexSessionID: sessionID, UpdatedAt: time.Now()}
	}
}

type tabTitleSyncTarget struct {
	InstanceID string
	Class      string
	PaneID     string
	Title      string
}

func (s *Server) syncCodexSessionTitle(ctx context.Context, sessionID, title string) {
	if title == "" {
		if session, ok, err := findCodexSession(sessionID); err == nil && ok {
			title = s.codexSessionDisplayTitle(session)
		}
	}
	targets := s.codexSessionTitleTargets(sessionID, title)
	s.syncTabTitleTargets(ctx, targets)
}

func (s *Server) codexSessionTitleTargets(sessionID, title string) []tabTitleSyncTarget {
	cfg := s.configSnapshot()
	classByInstance := make(map[string]string, len(cfg.Instances))
	for _, instance := range cfg.Instances {
		classByInstance[instance.ID] = instance.Class
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	targets := []tabTitleSyncTarget{}
	for key, input := range s.paneInputs {
		if input.CodexSessionID != sessionID {
			continue
		}
		instanceID, paneID, ok := splitPaneInputKey(key)
		if !ok {
			continue
		}
		class := classByInstance[instanceID]
		if class == "" {
			continue
		}
		targets = append(targets, tabTitleSyncTarget{
			InstanceID: instanceID,
			Class:      class,
			PaneID:     paneID,
			Title:      title,
		})
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].InstanceID != targets[j].InstanceID {
			return targets[i].InstanceID < targets[j].InstanceID
		}
		return targets[i].PaneID < targets[j].PaneID
	})
	return targets
}

func (s *Server) syncSessionTreeTitles(ctx context.Context, instance config.Instance, tree *sessionTree, force bool) {
	targets := make([]tabTitleSyncTarget, 0, len(tree.Panes))
	for _, pane := range tree.Panes {
		if pane.DisplayTitle == "" {
			continue
		}
		targets = append(targets, tabTitleSyncTarget{
			InstanceID: instance.ID,
			Class:      instance.Class,
			PaneID:     pane.PaneID,
			Title:      pane.DisplayTitle,
		})
	}
	s.syncTabTitleTargets(ctx, targets, force)
}

func (s *Server) syncTabTitleTargets(ctx context.Context, targets []tabTitleSyncTarget, force ...bool) {
	forceSync := len(force) > 0 && force[0]
	for _, target := range targets {
		s.syncTabTitle(ctx, target, forceSync)
	}
}

func (s *Server) syncTabTitle(ctx context.Context, target tabTitleSyncTarget, force ...bool) {
	if target.Class == "" || target.PaneID == "" {
		return
	}
	key := paneInputKey(target.InstanceID, target.PaneID)
	forceSync := len(force) > 0 && force[0]

	s.mu.RLock()
	lastTitle, alreadySynced := s.syncedTabTitles[key]
	s.mu.RUnlock()
	if !forceSync && alreadySynced && lastTitle == target.Title {
		return
	}

	if err := s.wezterm.SetTabTitle(ctx, target.Class, target.PaneID, target.Title); err != nil {
		s.logger.Warn("failed to sync wezterm tab title", "instance", target.InstanceID, "class", target.Class, "pane", target.PaneID, "error", err)
		return
	}

	s.mu.Lock()
	s.syncedTabTitles[key] = target.Title
	s.mu.Unlock()
}

func (s *Server) applyCodexSessionTitles(items []codexSessionItem) {
	cfg := s.configSnapshot()
	for i := range items {
		if title := cfg.CodexSessionTitles[items[i].ID]; title != "" {
			items[i].CustomTitle = title
		}
		items[i].DisplayTitle = s.codexSessionDisplayTitle(items[i])
	}
}

func (s *Server) codexSessionDisplayTitle(item codexSessionItem) string {
	cfg := s.configSnapshot()
	if title := cfg.CodexSessionTitles[item.ID]; title != "" {
		return title
	}
	return defaultCodexSessionTitle(item.Summary)
}

func (s *Server) codexSessionDisplayTitles(sessionIDs []string) map[string]string {
	cfg := s.configSnapshot()
	titles := make(map[string]string, len(sessionIDs))
	needDefault := map[string]bool{}
	for _, sessionID := range sessionIDs {
		if sessionID == "" {
			continue
		}
		if title := cfg.CodexSessionTitles[sessionID]; title != "" {
			titles[sessionID] = title
		} else {
			needDefault[sessionID] = true
		}
	}
	if len(needDefault) == 0 {
		return titles
	}
	sessions, err := codexSessionsFromHistory(0, "")
	if err != nil {
		s.logger.Debug("failed to load codex sessions for default titles", "error", err)
		return titles
	}
	for _, session := range sessions {
		if needDefault[session.ID] {
			if title := defaultCodexSessionTitle(session.Summary); title != "" {
				titles[session.ID] = title
			}
		}
	}
	return titles
}

func defaultCodexSessionTitle(summary string) string {
	summary = summarizePaneInput(summary, 0)
	if summary == "" {
		return ""
	}
	runes := []rune(summary)
	if len(runes) > 20 {
		return string(runes[:20])
	}
	return summary
}

func (s *Server) paneInputsSnapshot(instanceID string) map[string]paneInput {
	prefix := instanceID + "\x00"
	items := map[string]paneInput{}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for key, input := range s.paneInputs {
		if strings.HasPrefix(key, prefix) {
			items[strings.TrimPrefix(key, prefix)] = input
		}
	}
	return items
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
	return codexSessionsFromHistory(limit, "")
}

func codexSessionsFromHistory(limit int, query string) ([]codexSessionItem, error) {
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
	items := make([]codexSessionItem, 0, len(paths))
	query = strings.ToLower(strings.TrimSpace(query))
	for _, path := range paths {
		item, ok := readCodexSessionItem(path)
		if !ok {
			continue
		}
		if query != "" && !codexSessionMatchesQuery(item, path, query) {
			continue
		}
		items = append(items, item)
		if limit > 0 && len(items) >= limit {
			break
		}
	}
	return items, nil
}

func findCodexSession(sessionID string) (codexSessionItem, bool, error) {
	sessions, err := codexSessionsFromHistory(0, "")
	if err != nil {
		return codexSessionItem{}, false, err
	}
	for _, session := range sessions {
		if session.ID == sessionID {
			return session, true, nil
		}
	}
	return codexSessionItem{}, false, nil
}

func codexSessionMatchesQuery(item codexSessionItem, path, query string) bool {
	if query == "" {
		return true
	}
	for _, value := range []string{item.ID, item.CWD, item.Timestamp, item.Summary, item.CustomTitle, item.DisplayTitle, path} {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), query)
}

func (s *Server) recentCodexSessionsCached(limit int) ([]codexSessionItem, error) {
	const maxAge = 5 * time.Second
	now := time.Now()
	s.mu.RLock()
	if s.codexCacheLimit >= limit && now.Sub(s.codexCacheAt) < maxAge {
		end := limit
		if end > len(s.codexCache) {
			end = len(s.codexCache)
		}
		items := append([]codexSessionItem(nil), s.codexCache[:end]...)
		s.mu.RUnlock()
		return items, nil
	}
	s.mu.RUnlock()

	items, err := recentCodexSessions(limit)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.codexCache = append([]codexSessionItem(nil), items...)
	s.codexCacheAt = now
	s.codexCacheLimit = limit
	s.mu.Unlock()
	return append([]codexSessionItem(nil), items...), nil
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

func codexSessionIDFromCommand(text string) string {
	fields := strings.Fields(strings.TrimSpace(strings.ReplaceAll(text, "\r", "\n")))
	seenCodex := false
	for i, field := range fields {
		command := strings.Trim(strings.ToLower(field), `"'`)
		if command == "codex" || command == "codex.exe" {
			seenCodex = true
			continue
		}
		if seenCodex && strings.EqualFold(field, "resume") && i+1 < len(fields) {
			candidate := strings.Trim(fields[i+1], `"'`)
			if validCodexSessionID(candidate) {
				return candidate
			}
		}
	}
	return ""
}

func codexSessionIDFromText(text string) string {
	if sessionID := codexSessionIDFromCommand(text); sessionID != "" {
		return sessionID
	}
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r <= 32 || strings.ContainsRune(`"'()[]{}<>.,;:|`, r)
	})
	for i := len(fields) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(fields[i])
		if strings.Count(candidate, "-") < 4 {
			continue
		}
		if validCodexSessionID(candidate) {
			return candidate
		}
	}
	return ""
}

func validCodexSessionID(value string) bool {
	if len(value) < 8 || len(value) > 128 {
		return false
	}
	for _, ch := range value {
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '-' || ch == '_' {
			continue
		}
		return false
	}
	return true
}

func cwdKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(value), "file:") {
		if parsed, err := url.Parse(value); err == nil {
			value = parsed.Path
			if unescaped, err := url.PathUnescape(value); err == nil {
				value = unescaped
			}
			if len(value) >= 3 && value[0] == '/' && value[2] == ':' {
				value = value[1:]
			}
		}
	}
	value = strings.ReplaceAll(value, "/", `\`)
	value = filepath.Clean(value)
	value = strings.TrimRight(value, `\`)
	return strings.ToLower(value)
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
		if cfg.Token == "" || isLocalRequest(r) {
			s.recordClient(r)
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

func (s *Server) localOrAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if isLocalRequest(r) {
			next(w, r)
			return
		}
		s.auth(next)(w, r)
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
	current := s.configSnapshot()
	if next.CodexSessionTitles == nil {
		next.CodexSessionTitles = cloneStringMap(current.CodexSessionTitles)
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
	if cfg.CodexSessionTitles != nil {
		cfg.CodexSessionTitles = cloneStringMap(cfg.CodexSessionTitles)
	}
	return cfg
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
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
		if err != nil || value < 1 || value > 10000 {
			writeError(w, http.StatusBadRequest, errors.New("lines must be between 1 and 10000"))
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
		if pane.IsConfirm {
			tree.ConfirmCount++
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
	isWorking := isWorkingPane(raw)
	return paneSession{
		PaneID:           strconv.Itoa(raw.PaneID),
		WindowID:         raw.WindowID,
		TabID:            raw.TabID,
		Title:            raw.Title,
		CWD:              raw.CWD,
		Workspace:        raw.Workspace,
		IsActive:         raw.IsActive,
		IsWorking:        isWorking,
		IsConfirm:        !isWorking && isConfirmPane(raw),
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

func isConfirmPane(raw weztermPane) bool {
	original := strings.TrimSpace(raw.Title + " " + raw.TabTitle)
	lower := strings.ToLower(original)
	if strings.HasPrefix(strings.TrimSpace(raw.Title), "?") || strings.HasPrefix(strings.TrimSpace(raw.TabTitle), "?") ||
		strings.HasPrefix(strings.TrimSpace(raw.Title), "？") || strings.HasPrefix(strings.TrimSpace(raw.TabTitle), "？") {
		return true
	}
	for _, marker := range []string{
		"confirm",
		"approval",
		"approve",
		"waiting for input",
		"waiting input",
		"needs input",
		"plan mode",
		"planning",
	} {
		if strings.Contains(lower, marker) {
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
