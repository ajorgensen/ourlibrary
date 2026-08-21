package web

import (
	"errors"
	"net/http"

	"github.com/ajorgensen/ourlibrary/internal/database/store"
	"github.com/ajorgensen/ourlibrary/internal/web/templates"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (h *handler) showBook(w http.ResponseWriter, r *http.Request) {
	bookID, ok := parseUUID(r.PathValue("bookID"))
	if !ok {
		http.NotFound(w, r)
		return
	}

	user := userFromContext(r.Context())
	library, err := h.primaryLibrary(r, user.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	libraryID := pgtype.UUID{}
	if library != nil {
		libraryID = library.ID
	}

	book, err := h.queries.GetBook(r.Context(), store.GetBookParams{ID: bookID, LibraryID: libraryID})
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	h.render(w, r, http.StatusOK, templates.BookPage(user, library, book))
}

func parseUUID(value string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil || !id.Valid {
		return pgtype.UUID{}, false
	}
	return id, true
}
