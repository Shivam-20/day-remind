package web

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/shivam-20/day-remind/internal/config"
	"github.com/shivam-20/day-remind/internal/storage"
)

//go:embed all:static
var staticFiles embed.FS

type Server struct {
	cfg config.Config
	db  *storage.DB
}

func New(cfg config.Config, db *storage.DB) *Server {
	return &Server{cfg: cfg, db: db}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Post("/api/auth/signup", s.handleSignup)
	r.Post("/api/auth/login", s.handleLogin)
	r.Post("/api/auth/logout", s.handleLogout)

	r.Group(func(pr chi.Router) {
		pr.Use(s.requireSession)
		pr.Get("/api/me", s.handleMe)
		pr.Post("/api/me/notifications/pause", s.handlePauseNotifications)
		pr.Post("/api/me/notifications/resume", s.handleResumeNotifications)

		pr.Get("/api/reminders", s.handleListReminders)
		pr.Post("/api/reminders", s.handleCreateReminder)
		pr.Get("/api/reminders/{id}", s.handleGetReminder)
		pr.Patch("/api/reminders/{id}", s.handleUpdateReminder)
		pr.Delete("/api/reminders/{id}", s.handleDeleteReminder)
		pr.Post("/api/reminders/{id}/done", s.handleDoneReminder)
		pr.Post("/api/reminders/{id}/pause", s.handlePauseReminder)
		pr.Post("/api/reminders/{id}/resume", s.handleResumeReminder)
		pr.Post("/api/reminders/{id}/snooze", s.handleSnoozeReminder)
	})

	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(staticFS))
	r.Handle("/static/*", http.StripPrefix("/static/", fileServer))
	r.Get("/", s.serveIndex)
	r.Get("/portal", s.serveIndex)
	r.NotFound(s.serveIndex)

	return r
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	b, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "missing index", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
	_, _ = w.Write(b)
}

func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf(":%d", s.cfg.WebPort)
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	fmt.Printf("day-remind listening on http://localhost%s\n", addr)
	return srv.ListenAndServe()
}
