package web

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/a-h/templ"
	"github.com/ajorgensen/ourlibrary/internal/database/store"
	"github.com/ajorgensen/ourlibrary/internal/web/templates"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed assets/dist
var staticAssets embed.FS

type handler struct {
	logger  *slog.Logger
	pool    *pgxpool.Pool
	queries *store.Queries
}

func NewHandler(logger *slog.Logger, pool *pgxpool.Pool) http.Handler {
	h := &handler{
		logger:  logger,
		pool:    pool,
		queries: store.New(pool),
	}

	assets, err := fs.Sub(staticAssets, "assets/dist")
	if err != nil {
		panic(fmt.Sprintf("load static assets: %v", err))
	}

	mux := http.NewServeMux()
	mux.Handle("GET /{$}", h.requireUser(http.HandlerFunc(h.home)))
	mux.Handle("POST /books", h.requireUser(http.HandlerFunc(h.createBook)))
	mux.HandleFunc("GET /login", h.loginPage)
	mux.HandleFunc("POST /login", h.login)
	mux.HandleFunc("POST /logout", h.logout)
	mux.HandleFunc("GET /healthz", h.health)
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(assets)))

	return h.recoverPanics(mux)
}

func (h *handler) home(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	library, err := h.primaryLibrary(r, user.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	if library == nil {
		h.render(w, r, http.StatusOK, templates.Home(user, nil, nil))
		return
	}

	books, err := h.queries.ListBooks(r.Context(), library.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	h.render(w, r, http.StatusOK, templates.Home(user, library, books))
}

func (h *handler) createBook(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.formError(w, r, "Could not read the form. Try again.")
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	author := strings.TrimSpace(r.FormValue("author"))
	if title == "" || author == "" {
		h.formError(w, r, "A title and author are required.")
		return
	}

	user := userFromContext(r.Context())
	library, err := h.primaryLibrary(r, user.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	if library == nil {
		h.formError(w, r, "You must belong to a library before you can add books.")
		return
	}

	_, err = h.queries.CreateBook(r.Context(), store.CreateBookParams{
		LibraryID: library.ID,
		Title:     title,
		Author:    author,
	})
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	books, err := h.queries.ListBooks(r.Context(), library.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	w.Header().Set("HX-Trigger", "book-created")
	h.render(w, r, http.StatusCreated, templates.BookList(books))
}

func (h *handler) primaryLibrary(r *http.Request, userID pgtype.UUID) (*store.Library, error) {
	libraries, err := h.queries.ListLibrariesForUser(r.Context(), userID)
	if err != nil {
		return nil, err
	}
	if len(libraries) == 0 {
		return nil, nil
	}

	return &libraries[0], nil
}

func (h *handler) formError(w http.ResponseWriter, r *http.Request, message string) {
	w.Header().Set("HX-Retarget", "#form-error")
	w.Header().Set("HX-Reswap", "innerHTML")
	// HTMX swaps successful responses by default, so return the validation fragment as 200.
	h.render(w, r, http.StatusOK, templates.FormError(message))
}

func (h *handler) health(w http.ResponseWriter, r *http.Request) {
	if err := h.pool.Ping(r.Context()); err != nil {
		h.serverError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (h *handler) render(w http.ResponseWriter, r *http.Request, status int, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := component.Render(r.Context(), w); err != nil {
		h.logger.Error("render response", "method", r.Method, "path", r.URL.Path, "error", err)
	}
}

func (h *handler) serverError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}

func (h *handler) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				h.serverError(w, r, fmt.Errorf("panic: %v", recovered))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
