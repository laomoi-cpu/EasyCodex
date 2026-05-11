package server

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
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
	restart    func()
	updateJob  updateJobStatus
	logger     *slog.Logger
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
}

type settingsResponse struct {
	Config          config.Config `json:"config"`
	ConfigPath      string        `json:"configPath"`
	Network         netinfo.Info  `json:"network"`
	Version         string        `json:"version"`
	RestartRequired bool          `json:"restartRequired"`
	RestartFields   []string      `json:"restartFields,omitempty"`
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
	Instance string          `json:"instance"`
	Windows  []windowSession `json:"windows"`
	Panes    []paneSession   `json:"panes"`
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
	Workspace        string          `json:"workspace"`
	IsActive         bool            `json:"isActive"`
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
	return &Server{cfg: cfg, configPath: configPath, wezterm: wezterm, instances: instances, clients: map[string]clientConnection{}, logger: logger}, nil
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
	mux.HandleFunc("GET /api/settings", s.localOnly(s.settings))
	mux.HandleFunc("GET /api/connections", s.localOnly(s.connections))
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
	mux.HandleFunc("POST /api/instances/{instanceID}/panes/{paneID}/send", s.auth(s.sendText))
	mux.HandleFunc("DELETE /api/instances/{instanceID}/panes/{paneID}", s.auth(s.deletePane))
	mux.HandleFunc("POST /api/instances/{instanceID}/spawn", s.auth(s.spawn))
	return s.logRequests(mux)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	cfg := s.configSnapshot()
	network := netinfo.Inspect(cfg.Listen)
	writeOK(w, http.StatusOK, map[string]any{
		"service":    "easycodex-agent",
		"time":       time.Now().Format(time.RFC3339Nano),
		"lanEnabled": network.LANEnabled,
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
	s.writePairingConsole(w, baseURLs)
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
	writeOK(w, http.StatusOK, appConfigResponse{
		Instances: s.instanceResponses(),
		Defaults:  s.mobileDefaultsResponse(),
	})
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
