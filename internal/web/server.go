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
	"github.com/shivam-20/day-remind/internal/delivery"
	"github.com/shivam-20/day-remind/internal/secret"
	"github.com/shivam-20/day-remind/internal/storage"
)

//go:embed all:static
var staticFiles embed.FS

type Server struct {
	cfg config.Config
	db  *storage.DB
	// resolver is the same one the scheduler uses, so a test notification
	// exercises exactly the real delivery path.
	resolver *delivery.Resolver
	// box encrypts credentials on their way into the database.
	box *secret.Box
}

func New(cfg config.Config, db *storage.DB, resolver *delivery.Resolver, box *secret.Box) *Server {
	return &Server{cfg: cfg, db: db, resolver: resolver, box: box}
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

		pr.Get("/api/me/notify", s.handleGetNotifySettings)
		pr.Patch("/api/me/notify", s.handleUpdateNotifySettings)
		pr.Post("/api/me/notify/test", s.handleSendTestNotification)
		pr.Post("/api/me/telegram/link-code", s.handleTelegramLinkCode)
		pr.Post("/api/me/telegram/unlink", s.handleTelegramUnlink)
		pr.Patch("/api/me/notify/smtp", s.handleUpdateOwnSMTP)
		pr.Patch("/api/me/notify/telegram", s.handleUpdateOwnTelegram)
		pr.Post("/api/me/notify/telegram/discover", s.handleDiscoverOwnTelegramChat)
		pr.Get("/api/me/templates", s.handleGetOwnTemplates)
		pr.Put("/api/me/templates", s.handleUpdateOwnTemplates)
		pr.Post("/api/me/templates/preview", s.handleTemplatePreview)

		pr.Get("/api/reminders", s.handleListReminders)
		pr.Post("/api/reminders", s.handleCreateReminder)
		pr.Get("/api/reminders/{id}", s.handleGetReminder)
		pr.Patch("/api/reminders/{id}", s.handleUpdateReminder)
		pr.Delete("/api/reminders/{id}", s.handleDeleteReminder)
		pr.Post("/api/reminders/{id}/done", s.handleDoneReminder)
		pr.Post("/api/reminders/{id}/pause", s.handlePauseReminder)
		pr.Post("/api/reminders/{id}/resume", s.handleResumeReminder)
		pr.Post("/api/reminders/{id}/snooze", s.handleSnoozeReminder)

		pr.Group(func(ar chi.Router) {
			ar.Use(s.requireAdmin)
			ar.Get("/api/admin/stats", s.handleAdminStats)
			ar.Get("/api/admin/users", s.handleAdminListUsers)
			ar.Post("/api/admin/users/{id}/role", s.handleAdminSetRole)
			ar.Delete("/api/admin/users/{id}", s.handleAdminDeleteUser)
			ar.Post("/api/admin/users/{id}/notifications/pause", s.handleAdminPauseUser)
			ar.Post("/api/admin/users/{id}/notifications/resume", s.handleAdminResumeUser)

			ar.Get("/api/admin/settings", s.handleAdminGetSettings)
			ar.Patch("/api/admin/settings", s.handleAdminUpdateSettings)
			ar.Post("/api/admin/settings/test", s.handleAdminTestSettings)
			ar.Put("/api/admin/templates", s.handleAdminUpdateTemplates)
			ar.Post("/api/admin/templates/reset", s.handleAdminResetTemplates)
			ar.Post("/api/admin/templates/preview", s.handleTemplatePreview)
			ar.Get("/api/admin/deliveries", s.handleAdminDeliveryLog)
		})
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

// HTTPServer builds the configured server without starting it, so main can
// own the lifecycle and shut it down gracefully.
func (s *Server) HTTPServer() *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", s.cfg.WebPort),
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
}

// ListenAndServe is retained for callers that do not need graceful shutdown.
func (s *Server) ListenAndServe() error {
	return s.HTTPServer().ListenAndServe()
}
