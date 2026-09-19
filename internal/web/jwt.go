package web

import (
	"fmt"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/shivam-20/day-remind/internal/storage"
)

// gatewayClaims are the claims the home gateway signs into the auth_token cookie.
type gatewayClaims struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
	jwt.RegisteredClaims
}

// verifyGatewayJWT validates the shared auth_token JWT issued by the home
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
	user, err := s.db.GetUserByEmail(r.Context(), claims.Email)
	if err == nil {
		return user.ID, true
	}
	if err == storage.ErrNotFound {
		u, err := s.db.CreateUser(r.Context(), claims.Email, "", claims.Name, "")
		if err == nil {
			return u.ID, true
		}
		// A concurrent request may have created the user already — retry.
		if u2, err := s.db.GetUserByEmail(r.Context(), claims.Email); err == nil {
			return u2.ID, true
		}
	}
	return uuid.Nil, false
}
