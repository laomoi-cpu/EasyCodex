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
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"easycodex-agent/internal/config"
	"easycodex-agent/internal/netinfo"
)

type WezTerm interface {
	Launch(ctx context.Context, class string) (int, error)
	List(ctx context.Context, class string) (json.RawMessage, error)
	GetText(ctx context.Context, class, paneID string, lines int, escapes bool) (string, error)
	SendText(ctx context.Context, class, paneID, text string, noPaste bool) error
	Spawn(ctx context.Context, class, paneID, cwd string, newWindow bool, command []string) (string, error)
}

type Server struct {
	cfg       config.Config
	wezterm   WezTerm
	instances map[string]config.Instance
	logger    *slog.Logger
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
	return &Server{cfg: cfg, wezterm: wezterm, instances: instances, logger: logger}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/pairing", s.pairing)
	mux.HandleFunc("GET /pairing", s.pairingPage)
	mux.HandleFunc("GET /api/mobile-pair", s.mobilePair)
	mux.HandleFunc("GET /api/config", s.auth(s.appConfig))
	mux.HandleFunc("GET /api/instances", s.auth(s.instancesList))
	mux.HandleFunc("POST /api/instances/{instanceID}/launch", s.auth(s.launch))
	mux.HandleFunc("GET /api/instances/{instanceID}/sessions", s.auth(s.sessions))
	mux.HandleFunc("GET /api/instances/{instanceID}/panes/{paneID}/text", s.auth(s.paneText))
	mux.HandleFunc("GET /api/instances/{instanceID}/panes/{paneID}/snapshot", s.auth(s.paneSnapshot))
	mux.HandleFunc("POST /api/instances/{instanceID}/panes/{paneID}/send", s.auth(s.sendText))
	mux.HandleFunc("POST /api/instances/{instanceID}/spawn", s.auth(s.spawn))
	return s.logRequests(mux)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	network := netinfo.Inspect(s.cfg.Listen)
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
	writeOK(w, http.StatusOK, pairingResponse{
		Service:     "easycodex-agent",
		Network:     netinfo.Inspect(s.cfg.Listen),
		Token:       s.cfg.Token,
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
	network := netinfo.Inspect(s.cfg.Listen)
	baseURLs := append([]string(nil), network.LANURLs...)
	if len(baseURLs) == 0 {
		baseURLs = append(baseURLs, network.LocalURL)
	} else {
		baseURLs = append(baseURLs, network.LocalURL)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!doctype html>
<html><head><meta charset="utf-8"><title>EasyCodex Pairing</title>
<style>body{font-family:Segoe UI,Arial,sans-serif;margin:32px;background:#f6f7f9;color:#111827}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(360px,1fr));gap:16px}.panel{background:white;border:1px solid #d1d5db;border-radius:8px;padding:24px}.qr{width:320px;height:320px;border:1px solid #e5e7eb}.label{font-size:12px;color:#6b7280;margin-top:18px}.value{font-family:Consolas,monospace;word-break:break-all;background:#f3f4f6;padding:10px;border-radius:6px}.warn{color:#92400e;background:#fffbeb;border:1px solid #f59e0b;padding:10px;border-radius:6px}.hint{max-width:760px}</style>
</head><body><h1>EasyCodex Pairing</h1><p class="hint">Scan the QR code whose address is on the same Wi-Fi network as your phone. VPN and virtual adapter addresses may not work.</p><div class="grid">`)
	for _, baseURL := range baseURLs {
		pairURL := baseURL + "/api/mobile-pair?code=" + url.QueryEscape(s.mobilePairCode())
		deepLink := "easycodex://pair?url=" + url.QueryEscape(pairURL)
		qrURL := "https://api.qrserver.com/v1/create-qr-code/?size=320x320&data=" + url.QueryEscape(deepLink)
		fmt.Fprintf(w, `<div class="panel"><img class="qr" src="%s" alt="Pairing QR"><div class="label">Phone Base URL</div><div class="value">%s</div><div class="label">Pair Link</div><div class="value">%s</div></div>`, qrURL, baseURL, deepLink)
	}
	fmt.Fprint(w, `</div><p class="warn">If the phone cannot connect, set listen to 0.0.0.0:8765 and allow the Windows firewall.</p></body></html>`)
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
	writeOK(w, http.StatusOK, map[string]any{
		"baseUrl":  baseURL,
		"token":    s.cfg.Token,
		"defaults": s.mobileDefaultsResponse(),
	})
}

func (s *Server) mobilePairCode() string {
	sum := sha256.Sum256([]byte(s.cfg.Token + "|" + s.cfg.Listen))
	return hex.EncodeToString(sum[:])[:12]
}

func (s *Server) appConfig(w http.ResponseWriter, r *http.Request) {
	writeOK(w, http.StatusOK, appConfigResponse{
		Instances: s.instanceResponses(),
		Defaults:  s.mobileDefaultsResponse(),
	})
}

func (s *Server) mobileDefaultsResponse() mobileDefaultsResponse {
	command := append([]string(nil), s.cfg.MobileDefaults.Command...)
	return mobileDefaultsResponse{
		InstanceID: s.cfg.MobileDefaults.InstanceID,
		CWD:        s.cfg.MobileDefaults.CWD,
		Command:    command,
	}
}

func (s *Server) instanceResponses() []instanceResponse {
	items := make([]instanceResponse, 0, len(s.cfg.Instances))
	for _, instance := range s.cfg.Instances {
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
	instance, ok := s.instances[id]
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("unknown instance %q", id))
		return config.Instance{}, false
	}
	return instance, true
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.Token == "" {
			next(w, r)
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" {
			token = r.Header.Get("X-EasyCodex-Token")
		}
		if token != s.cfg.Token {
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}
		next(w, r)
	}
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.logger.Info("request", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
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
