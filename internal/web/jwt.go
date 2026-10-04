package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/storage"
)

// ssoAdminRoles are the Launchpad roles that map to day-remind admin.
var ssoAdminRoles = map[string]bool{"owner": true, "admin": true}

// ungatedPathPrefixes mirror the paths the home Caddy proxies WITHOUT
// forward_auth (~/apps/caddy/Caddyfile). Caddy also strips client X-Auth-*
// headers, but if that ever regresses these requests would carry forged
// identity, so they are never trusted as identity here either.
var ungatedPathPrefixes = []string{
	"/api/v1/",
	"/api/health",
	"/api/webhooks/telegram",
	"/api/webhooks/email-ingest",
}

func isUngatedPath(path string) bool {
	for _, p := range ungatedPathPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// gatewayClaims are the claims the Launchpad (home) gateway signs into the
// auth_token cookie and exposes as X-Auth-* headers.
type gatewayClaims struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
	jwt.RegisteredClaims
}

// verifyGatewayJWT validates the shared auth_token JWT issued by the Launchpad
// gateway. The same AUTH_JWT_SECRET must be configured on the home service.
func verifyGatewayJWT(token, secret, issuer string) (*gatewayClaims, error) {
	if token == "" || secret == "" {
		return nil, fmt.Errorf("missing token or secret")
	}
	claims := &gatewayClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(issuer))
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// resolveGatewayUser finds (or provisions) the day-remind user for an SSO
// identity, and keeps the admin flag in sync with the SSO role so a Launchpad
// "owner"/"admin" grants day-remind admin without a restart.
func (s *Server) resolveGatewayUser(ctx context.Context, email, name, role string) (uuid.UUID, bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return uuid.Nil, false
	}
	user, err := s.db.GetUserByEmail(ctx, email)
	if err == storage.ErrNotFound {
		created, cerr := s.db.CreateUser(ctx, email, "", name, "")
		if cerr != nil {
			// A concurrent request may have created the user already — retry.
			if u2, gerr := s.db.GetUserByEmail(ctx, email); gerr == nil {
				user = u2
			} else {
				return uuid.Nil, false
			}
		} else {
			user = created
		}
	} else if err != nil {
		return uuid.Nil, false
	}
	// Mirror the Launchpad role both ways so revoking admin there revokes it
	// here too. An empty role carries no information, so leave the flag as is.
	// SetUserAdmin never demotes hardcoded / ADMIN_EMAILS admins.
	if role = strings.ToLower(strings.TrimSpace(role)); role != "" {
		if wantAdmin := ssoAdminRoles[role]; wantAdmin != user.IsAdmin {
			_ = s.db.SetUserAdmin(ctx, user.ID, wantAdmin)
		}
	}
	return user.ID, true
}

// userFromGateway resolves a day-remind identity from the gateway auth_token
// cookie. Existing day-remind users are matched by email; if none exists, a
// gateway-only account (no password hash) is provisioned on the fly so the
// shared login also grants access to day-remind features.
func (s *Server) userFromGateway(r *http.Request) (uuid.UUID, bool) {
	c, err := r.Cookie(s.cfg.GatewayCookie)
	if err != nil || c.Value == "" {
		return uuid.Nil, false
	}
	claims, err := verifyGatewayJWT(c.Value, s.cfg.AuthJWTSecret, s.cfg.JWTIssuer)
	if err != nil {
		return uuid.Nil, false
	}
	return s.resolveGatewayUser(r.Context(), claims.Email, claims.Name, claims.Role)
}

// userFromHeaders resolves identity from the X-Auth-* headers that Caddy's
// forward_auth copies from the Launchpad /auth/verify response. Only trusted
// when TRUST_AUTH_HEADERS is enabled (the app is only reachable through Caddy,
// which overwrites these headers on a successful check).
func (s *Server) userFromHeaders(r *http.Request) (uuid.UUID, bool) {
	if !s.cfg.TrustAuthHeaders || isUngatedPath(r.URL.Path) {
		return uuid.Nil, false
	}
	email := strings.TrimSpace(r.Header.Get("X-Auth-Email"))
	if email == "" {
		return uuid.Nil, false
	}
	return s.resolveGatewayUser(r.Context(), email, r.Header.Get("X-Auth-Name"), r.Header.Get("X-Auth-Role"))
}

// syncAdminFromHeaders mirrors the live Launchpad role onto a user signed in
// with day-remind's own session, so a demotion at the IdP takes effect here
// without waiting for that session to expire. Only applies when the trusted
// SSO identity is the same person (matching email).
func (s *Server) syncAdminFromHeaders(r *http.Request, userID uuid.UUID) {
	if !s.cfg.TrustAuthHeaders || isUngatedPath(r.URL.Path) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Auth-Email")))
	role := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Auth-Role")))
	if email == "" || role == "" {
		return
	}
	user, err := s.db.GetUserByID(r.Context(), userID)
	if err != nil || strings.ToLower(user.Email) != email {
		return
	}
	if wantAdmin := ssoAdminRoles[role]; wantAdmin != user.IsAdmin {
		_ = s.db.SetUserAdmin(r.Context(), user.ID, wantAdmin)
	}
}
