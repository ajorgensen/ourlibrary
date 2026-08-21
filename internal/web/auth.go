package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ajorgensen/ourlibrary/internal/account"
	"github.com/ajorgensen/ourlibrary/internal/database/store"
	"github.com/ajorgensen/ourlibrary/internal/web/templates"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	sessionCookieName = "ourlibrary_session"
	sessionDuration   = 30 * 24 * time.Hour
)

type userContextKey struct{}

func (h *handler) loginPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, http.StatusOK, templates.Login("", ""))
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.render(w, r, http.StatusBadRequest, templates.Login("", "Could not read the form. Try again."))
		return
	}

	input := strings.TrimSpace(r.FormValue("email"))
	email, err := account.NormalizeEmail(input)
	if err != nil {
		h.render(w, r, http.StatusUnprocessableEntity, templates.Login(input, "Enter a valid email address."))
		return
	}

	user, err := h.queries.GetUserByEmail(r.Context(), email)
	if errors.Is(err, pgx.ErrNoRows) {
		h.render(w, r, http.StatusUnauthorized, templates.Login(input, "No user was found for that email address."))
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		h.serverError(w, r, err)
		return
	}

	tokenHash := sha256.Sum256(token)
	expiresAt := time.Now().Add(sessionDuration)
	if err := h.queries.CreateSession(r.Context(), store.CreateSessionParams{
		UserID:    user.ID,
		TokenHash: tokenHash[:],
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	}); err != nil {
		h.serverError(w, r, err)
		return
	}

	h.setSessionCookie(w, r, base64.RawURLEncoding.EncodeToString(token), expiresAt)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	if tokenHash, ok := sessionTokenHash(r); ok {
		if err := h.queries.DeleteSession(r.Context(), tokenHash); err != nil {
			h.serverError(w, r, err)
			return
		}
	}

	h.clearSessionCookie(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *handler) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenHash, ok := sessionTokenHash(r)
		if !ok {
			h.redirectToLogin(w, r)
			return
		}

		user, err := h.queries.GetUserBySessionToken(r.Context(), tokenHash)
		if errors.Is(err, pgx.ErrNoRows) {
			h.clearSessionCookie(w, r)
			h.redirectToLogin(w, r)
			return
		}
		if err != nil {
			h.serverError(w, r, err)
			return
		}

		if err := h.queries.TouchSession(r.Context(), tokenHash); err != nil {
			h.serverError(w, r, err)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey{}, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func userFromContext(ctx context.Context) store.User {
	return ctx.Value(userContextKey{}).(store.User)
}

func sessionTokenHash(r *http.Request) ([]byte, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, false
	}

	token, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil || len(token) != 32 {
		return nil, false
	}

	tokenHash := sha256.Sum256(token)
	return tokenHash[:], true
}

func (h *handler) setSessionCookie(w http.ResponseWriter, r *http.Request, value string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(sessionDuration.Seconds()),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *handler) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *handler) redirectToLogin(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/login")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
