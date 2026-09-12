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
	r.Post("/api/auth/forgot-password", s.handleForgotPassword)
	r.Post("/api/auth/reset-password", s.handleResetPassword)
	r.Post("/api/webhooks/telegram", s.handleTelegramWebhook)
	r.Post("/api/webhooks/email-ingest", s.handleEmailIngest)
	r.Post("/api/webhooks/email-ingest/{token}", s.handleEmailIngest)

	r.Group(func(pr chi.Router) {
		pr.Use(s.requireSession)
		pr.Get("/api/me", s.handleMe)
		pr.Post("/api/me/notifications/pause", s.handlePauseNotifications)
		pr.Post("/api/me/notifications/resume", s.handleResumeNotifications)
		pr.Get("/api/me/notification-channels", s.handleGetNotificationChannels)
		pr.Patch("/api/me/notification-channels", s.handleUpdateNotificationChannels)
		pr.Post("/api/me/telegram/link", s.handleTelegramLink)
		pr.Patch("/api/me/digest-settings", s.handleUpdateDigestSettings)
		pr.Get("/api/me/email-ingest", s.handleGetEmailIngestAddress)
		pr.Post("/api/me/2fa/setup", s.handleTOTPSetup)
		pr.Post("/api/me/2fa/enable", s.handleTOTPEnable)
		pr.Post("/api/me/2fa/disable", s.handleTOTPDisable)

		pr.Get("/api/notifications", s.handleListNotifications)
		pr.Get("/api/snooze/presets", s.handleSnoozePresets)
		pr.Get("/api/stats/dashboard", s.handleDashboardStats)
		pr.Get("/api/calendar", s.handleCalendar)

		pr.Get("/api/lists", s.handleListLists)
		pr.Post("/api/lists", s.handleCreateList)
		pr.Get("/api/lists/{id}", s.handleGetList)
		pr.Patch("/api/lists/{id}", s.handleUpdateList)
		pr.Delete("/api/lists/{id}", s.handleDeleteList)
		pr.Post("/api/lists/{id}/pause", s.handlePauseList)
		pr.Post("/api/lists/{id}/resume", s.handleResumeList)

		pr.Get("/api/tags", s.handleListTags)
		pr.Post("/api/tags", s.handleCreateTag)
		pr.Patch("/api/tags/{id}", s.handleUpdateTag)
		pr.Delete("/api/tags/{id}", s.handleDeleteTag)

		pr.Get("/api/templates", s.handleListTemplates)
		pr.Post("/api/templates", s.handleCreateTemplate)
		pr.Post("/api/templates/{id}/use", s.handleUseTemplate)

		pr.Get("/api/groups", s.handleListGroups)
		pr.Post("/api/groups", s.handleCreateGroup)
		pr.Get("/api/groups/{id}", s.handleGetGroup)
		pr.Patch("/api/groups/{id}", s.handleUpdateGroup)
		pr.Delete("/api/groups/{id}", s.handleDeleteGroup)
		pr.Post("/api/groups/{id}/invite", s.handleInviteGroupMember)

		pr.Get("/api/api-keys", s.handleListAPIKeys)
		pr.Post("/api/api-keys", s.handleCreateAPIKey)
		pr.Delete("/api/api-keys/{id}", s.handleRevokeAPIKey)

		pr.Get("/api/webhooks", s.handleListWebhooks)
		pr.Post("/api/webhooks", s.handleCreateWebhook)
		pr.Get("/api/webhooks/{id}", s.handleGetWebhook)
		pr.Patch("/api/webhooks/{id}", s.handleUpdateWebhook)
		pr.Delete("/api/webhooks/{id}", s.handleDeleteWebhook)

		pr.Get("/api/export/json", s.handleExportJSON)
		pr.Get("/api/export/ical", s.handleExportICal)
		pr.Post("/api/import/json/preview", s.handleImportJSONPreview)
		pr.Post("/api/import/json", s.handleImportJSON)
		pr.Post("/api/import/csv", s.handleImportCSV)
		pr.Post("/api/import/ical", s.handleImportICal)

		pr.Get("/api/integrations/google", s.handleGetGoogleIntegration)
		pr.Post("/api/integrations/google/connect", s.handleGoogleConnect)
		pr.Get("/api/integrations/google/callback", s.handleGoogleCallback)
		pr.Delete("/api/integrations/google", s.handleGoogleDisconnect)
		pr.Post("/api/integrations/google/sync", s.handleGoogleSync)
		pr.Post("/api/integrations/google/calendar/pause", s.handleGoogleCalendarPause)
		pr.Post("/api/integrations/google/calendar/resume", s.handleGoogleCalendarResume)
		pr.Post("/api/integrations/google/gmail/pause", s.handleGoogleGmailPause)
		pr.Post("/api/integrations/google/gmail/resume", s.handleGoogleGmailResume)
		pr.Get("/api/integrations/google/gmail/settings", s.handleGetGmailSettings)
		pr.Put("/api/integrations/google/gmail/settings", s.handleUpdateGmailSettings)

		pr.Post("/api/push/subscribe", s.handlePushSubscribe)
		pr.Post("/api/reminders/parse", s.handleParseReminder)

		pr.Get("/api/reminders", s.handleListReminders)
		pr.Post("/api/reminders", s.handleCreateReminder)
		pr.Get("/api/reminders/{id}", s.handleGetReminder)
		pr.Get("/api/reminders/{id}/deliveries", s.handleListReminderDeliveries)
		pr.Patch("/api/reminders/{id}", s.handleUpdateReminder)
		pr.Delete("/api/reminders/{id}", s.handleDeleteReminder)
		pr.Post("/api/reminders/{id}/done", s.handleDoneReminder)
		pr.Post("/api/reminders/{id}/pause", s.handlePauseReminder)
		pr.Post("/api/reminders/{id}/resume", s.handleResumeReminder)
		pr.Post("/api/reminders/{id}/snooze", s.handleSnoozeReminder)
		pr.Post("/api/reminders/{id}/share", s.handleCreateShare)
		pr.Get("/api/reminders/{id}/shares", s.handleListShares)
		pr.Get("/api/reminders/{id}/comments", s.handleListComments)
		pr.Post("/api/reminders/{id}/comments", s.handleCreateComment)
		pr.Get("/api/reminders/{id}/subtasks", s.handleListSubtasks)
		pr.Post("/api/reminders/{id}/subtasks", s.handleCreateSubtask)
		pr.Post("/api/reminders/{id}/subtasks/reorder", s.handleReorderSubtasks)
		pr.Patch("/api/reminders/{id}/subtasks/{subId}", s.handleUpdateSubtask)
		pr.Delete("/api/reminders/{id}/subtasks/{subId}", s.handleDeleteSubtask)

		pr.Post("/api/shares/{id}/accept", s.handleAcceptShare)
	})

	r.Route("/api/v1", func(v1 chi.Router) {
		v1.Use(s.requireAPIKey)
		v1.Get("/reminders", s.handleListReminders)
		v1.Post("/reminders", s.handleCreateReminder)
		v1.Get("/reminders/{id}", s.handleGetReminder)
		v1.Patch("/reminders/{id}", s.handleUpdateReminder)
		v1.Delete("/reminders/{id}", s.handleDeleteReminder)
		v1.Post("/reminders/{id}/done", s.handleDoneReminder)
		v1.Post("/reminders/{id}/pause", s.handlePauseReminder)
		v1.Post("/reminders/{id}/resume", s.handleResumeReminder)
		v1.Post("/reminders/{id}/snooze", s.handleSnoozeReminder)
	})

	r.Group(func(ar chi.Router) {
		ar.Use(s.requireSession)
		ar.Use(s.requireAdmin)

		ar.Get("/api/admin/stats", s.handleAdminStats)

		ar.Get("/api/admin/settings/delivery", s.handleAdminGetDeliverySettings)
		ar.Patch("/api/admin/settings/delivery", s.handleAdminUpdateDeliverySettings)
		ar.Post("/api/admin/settings/delivery/test", s.handleAdminTestDelivery)

		ar.Get("/api/admin/users", s.handleAdminListUsers)
		ar.Post("/api/admin/users", s.handleAdminCreateUser)
		ar.Get("/api/admin/users/{id}", s.handleAdminGetUser)
		ar.Patch("/api/admin/users/{id}", s.handleAdminUpdateUser)
		ar.Delete("/api/admin/users/{id}", s.handleAdminDeleteUser)
		ar.Delete("/api/admin/users/{id}/sessions", s.handleAdminRevokeUserSessions)

		ar.Get("/api/admin/reminders", s.handleAdminListReminders)
		ar.Post("/api/admin/reminders", s.handleAdminCreateReminder)
		ar.Get("/api/admin/reminders/{id}", s.handleAdminGetReminder)
		ar.Patch("/api/admin/reminders/{id}", s.handleAdminUpdateReminder)
		ar.Delete("/api/admin/reminders/{id}", s.handleAdminDeleteReminder)
		ar.Post("/api/admin/reminders/{id}/done", s.handleAdminDoneReminder)
		ar.Post("/api/admin/reminders/{id}/pause", s.handleAdminPauseReminder)
		ar.Post("/api/admin/reminders/{id}/resume", s.handleAdminResumeReminder)
		ar.Post("/api/admin/reminders/{id}/snooze", s.handleAdminSnoozeReminder)
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
	if s.cfg.BasePath != "" {
		base := "/" + s.cfg.BasePath + "/"
		b = []byte(strings.Replace(string(b), "<head>", "<head><base href=\""+base+"\">", 1))
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
