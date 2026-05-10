package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"easycodex-agent/internal/config"
)

type WezTerm interface {
	Launch(ctx context.Context, class string) error
	List(ctx context.Context, class string) (json.RawMessage, error)
	GetText(ctx context.Context, class, paneID string, lines int, escapes bool) (string, error)
	SendText(ctx context.Context, class, paneID, text string, noPaste bool) error
	Spawn(ctx context.Context, class, cwd string, newWindow bool, command []string) (string, error)
}

type Server struct {
	cfg       config.Config
	wezterm   WezTerm
	instances map[string]config.Instance
	logger    *slog.Logger
}

func New(cfg config.Config, wezterm WezTerm, logger *slog.Logger) (*Server, error) {
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
	mux.HandleFunc("GET /api/instances", s.auth(s.instancesList))
	mux.HandleFunc("POST /api/instances/{instanceID}/launch", s.auth(s.launch))
	mux.HandleFunc("GET /api/instances/{instanceID}/sessions", s.auth(s.sessions))
	mux.HandleFunc("GET /api/instances/{instanceID}/panes/{paneID}/text", s.auth(s.paneText))
	mux.HandleFunc("POST /api/instances/{instanceID}/panes/{paneID}/send", s.auth(s.sendText))
	mux.HandleFunc("POST /api/instances/{instanceID}/spawn", s.auth(s.spawn))
	return s.logRequests(mux)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "easycodex-agent",
	})
}

func (s *Server) instancesList(w http.ResponseWriter, r *http.Request) {
	type item struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Class string `json:"class"`
	}
	items := make([]item, 0, len(s.cfg.Instances))
	for _, instance := range s.cfg.Instances {
		items = append(items, item{
			ID:    instance.ID,
			Name:  instance.Name,
			Class: instance.Class,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"instances": items})
}

func (s *Server) launch(w http.ResponseWriter, r *http.Request) {
	instance, ok := s.instance(w, r)
	if !ok {
		return
	}
	if err := s.wezterm.Launch(r.Context(), instance.Class); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"instance": instance.ID,
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

	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("invalid wezterm list json: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"instance": instance.ID,
		"sessions": payload,
	})
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

	lines := 200
	if raw := r.URL.Query().Get("lines"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 5000 {
			writeError(w, http.StatusBadRequest, errors.New("lines must be between 1 and 5000"))
			return
		}
		lines = value
	}
	escapes := parseBool(r.URL.Query().Get("escapes"))

	text, err := s.wezterm.GetText(r.Context(), instance.Class, paneID, lines, escapes)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"instance": instance.ID,
		"paneId":   paneID,
		"text":     text,
	})
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

	var body struct {
		Text    string `json:"text"`
		NoPaste bool   `json:"noPaste"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.Text == "" {
		writeError(w, http.StatusBadRequest, errors.New("text is required"))
		return
	}

	if err := s.wezterm.SendText(r.Context(), instance.Class, paneID, body.Text, body.NoPaste); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) spawn(w http.ResponseWriter, r *http.Request) {
	instance, ok := s.instance(w, r)
	if !ok {
		return
	}

	var body struct {
		CWD       string   `json:"cwd"`
		NewWindow bool     `json:"newWindow"`
		Command   []string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	paneID, err := s.wezterm.Spawn(r.Context(), instance.Class, body.CWD, body.NewWindow, body.Command)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"paneId": paneID,
	})
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

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{
		"ok":    false,
		"error": err.Error(),
	})
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

func parseBool(value string) bool {
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
