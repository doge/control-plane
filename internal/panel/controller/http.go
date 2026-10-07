package controller

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/example/control-plane/internal/models"
)

type contextKey string

const userKey contextKey = "user"

func (a *App) Handler() http.Handler {
	controller := &Controller{app: a, service: a.service}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/setup/status", controller.setupStatus)
	mux.Handle("/api/setup", a.rateLimit("setup", 5, 15*time.Minute)(http.HandlerFunc(controller.setup)))
	mux.Handle("/api/setup/verify", a.rateLimit("setup-verify", 10, 15*time.Minute)(http.HandlerFunc(controller.setupVerify)))
	mux.Handle("/api/auth/login", a.rateLimit("login", 10, 15*time.Minute)(http.HandlerFunc(controller.login)))
	mux.Handle("/api/auth/login/otp", a.rateLimit("login-otp", 10, 15*time.Minute)(http.HandlerFunc(controller.loginOTP)))
	mux.Handle("/api/auth/profile", a.auth(a.rateLimit("profile", 10, 15*time.Minute)(http.HandlerFunc(controller.accountSecurity))))
	mux.Handle("/api/auth/password", a.auth(a.rateLimit("password", 10, 15*time.Minute)(http.HandlerFunc(controller.accountSecurity))))
	mux.Handle("/api/auth/totp", a.auth(a.rateLimit("totp", 10, 15*time.Minute)(http.HandlerFunc(controller.accountSecurity))))
	mux.Handle("/api/auth/totp/enroll", a.auth(a.rateLimit("totp-enroll", 5, 15*time.Minute)(http.HandlerFunc(controller.accountSecurity))))
	mux.Handle("/api/auth/totp/verify", a.auth(a.rateLimit("totp-verify", 10, 15*time.Minute)(http.HandlerFunc(controller.accountSecurity))))
	mux.Handle("/api/auth/totp/remove", a.auth(a.rateLimit("totp-remove", 10, 15*time.Minute)(http.HandlerFunc(controller.accountSecurity))))
	mux.Handle("/api/auth/totp/recovery-codes", a.auth(a.rateLimit("totp-recovery", 10, 15*time.Minute)(http.HandlerFunc(controller.accountSecurity))))
	mux.Handle("/api/settings/transport", a.auth(http.HandlerFunc(controller.transportSettings)))
	mux.HandleFunc("/api/auth/logout", controller.logout)
	mux.HandleFunc("/api/auth/me", controller.me)
	mux.Handle("/api/dashboard", a.auth(http.HandlerFunc(controller.dashboard), "dashboard.view"))
	mux.Handle("/api/configs", a.auth(http.HandlerFunc(controller.configs)))
	mux.Handle("/api/configs/", a.auth(http.HandlerFunc(controller.configByID)))
	mux.Handle("/api/nodes", a.auth(http.HandlerFunc(controller.nodes)))
	mux.Handle("/api/nodes/", a.auth(http.HandlerFunc(controller.nodeByID)))
	mux.Handle("/api/servers", a.auth(http.HandlerFunc(controller.servers)))
	mux.Handle("/api/servers/", a.auth(http.HandlerFunc(controller.serverOps)))
	mux.Handle("/api/admin/users", a.auth(http.HandlerFunc(controller.users)))
	mux.Handle("/api/admin/users/", a.auth(http.HandlerFunc(controller.userByID)))
	mux.Handle("/api/admin/audit", a.auth(http.HandlerFunc(controller.audit), "audit.view"))
	mux.Handle("/api/roles", a.auth(http.HandlerFunc(controller.roles)))
	mux.Handle("/api/roles/", a.auth(http.HandlerFunc(controller.roles)))
	mux.HandleFunc("/api/node/connect", a.nodeConnect)
	mux.HandleFunc("/api/health", controller.health)
	return a.security(a.staticFallback(mux))
}
func (a *App) staticFallback(api http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			api.ServeHTTP(w, r)
			return
		}
		p := staticPath(a.cfg.StaticDir, r.URL.Path)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			http.ServeFile(w, r, p)
			return
		}
		http.ServeFile(w, r, staticPath(a.cfg.StaticDir, "index.html"))
	})
}
func (a *App) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
		if secure && a.httpsRequired.Load() {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		if a.httpsRequired.Load() && !secure {
			w.Header().Set("Location", "https://"+r.Host+r.URL.RequestURI())
			w.WriteHeader(http.StatusPermanentRedirect)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (a *App) auth(next http.Handler, roles ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := r.Header.Get("Authorization")
		if strings.HasPrefix(tok, "Bearer ") {
			tok = strings.TrimPrefix(tok, "Bearer ")
		}
		if tok == "" {
			if c, e := r.Cookie("session"); e == nil {
				tok = c.Value
			}
		}
		u, err := a.service.Authenticate(r.Context(), tok, time.Now())
		if err != nil {
			writeJSON(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
		if !hasPermissions(u, roles) {
			writeJSON(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

func requirePermission(w http.ResponseWriter, r *http.Request, permission string) bool {
	user, _ := r.Context().Value(userKey).(models.User)
	if userHasPermission(user, permission) {
		return true
	}
	writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission required: " + permission})
	return false
}
