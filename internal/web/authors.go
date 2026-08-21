package web

import (
	"fmt"
	"net/http"

	"github.com/ajorgensen/ourlibrary/internal/database/store"
	"github.com/ajorgensen/ourlibrary/internal/web/templates"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (h *handler) showAuthor(w http.ResponseWriter, r *http.Request) {
	h.renderAuthorPage(w, r, http.StatusOK, "")
}

func (h *handler) refreshAuthor(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("authorKey")
	if !isOpenLibraryKey(key, 'A') {
		http.NotFound(w, r)
		return
	}

	author, works, err := h.authorFinder.Find(r.Context(), key)
	if err != nil {
		h.logger.Error("refresh author", "author_key", key, "error", err)
		h.renderAuthorPage(w, r, http.StatusBadGateway, "Open Library is unavailable. Showing the books already cached.")
		return
	}
	if err := h.cacheAuthor(r, author, works); err != nil {
		h.serverError(w, r, err)
		return
	}

	http.Redirect(w, r, "/authors/"+key, http.StatusSeeOther)
}

func (h *handler) cacheAuthor(r *http.Request, metadata authorMetadata, works []bookMetadata) error {
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		return fmt.Errorf("begin author refresh: %w", err)
	}
	defer tx.Rollback(r.Context())
	queries := h.queries.WithTx(tx)

	author, err := queries.UpsertOpenLibraryAuthor(r.Context(), store.UpsertOpenLibraryAuthorParams{
		OpenLibraryAuthorKey: text(metadata.Key),
		Name:                 metadata.Name,
	})
	if err != nil {
		return fmt.Errorf("upsert refreshed author: %w", err)
	}
	author, err = queries.UpdateAuthorMetadata(r.Context(), store.UpdateAuthorMetadataParams{
		OpenLibraryAuthorKey: text(metadata.Key),
		Name:                 metadata.Name,
		Bio:                  text(metadata.Bio),
		BirthDate:            text(metadata.BirthDate),
		DeathDate:            text(metadata.DeathDate),
	})
	if err != nil {
		return fmt.Errorf("update author metadata: %w", err)
	}

	for _, metadata := range works {
		book, upsertErr := upsertOpenLibraryBook(r.Context(), queries, metadata)
		if upsertErr != nil {
			return upsertErr
		}
		if err := queries.LinkBookAuthor(r.Context(), store.LinkBookAuthorParams{BookID: book.ID, AuthorID: author.ID}); err != nil {
			return fmt.Errorf("link discovered work: %w", err)
		}
	}
	if err := queries.MarkAuthorWorksSynced(r.Context(), text(metadata.Key)); err != nil {
		return fmt.Errorf("mark author works synced: %w", err)
	}
	if err := tx.Commit(r.Context()); err != nil {
		return fmt.Errorf("commit author refresh: %w", err)
	}

	return nil
}

func (h *handler) addAuthorBook(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("authorKey")
	if !isOpenLibraryKey(key, 'A') {
		http.NotFound(w, r)
		return
	}

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
	if library == nil {
		http.Error(w, "You must belong to a library before you can add books.", http.StatusForbidden)
		return
	}

	if err := h.queries.AddAuthorBookToLibrary(r.Context(), store.AddAuthorBookToLibraryParams{
		OpenLibraryAuthorKey: text(key),
		LibraryID:            library.ID,
		AddedByUserID:        user.ID,
		BookID:               bookID,
	}); err != nil {
		h.serverError(w, r, err)
		return
	}

	http.Redirect(w, r, "/authors/"+key, http.StatusSeeOther)
}

func (h *handler) renderAuthorPage(w http.ResponseWriter, r *http.Request, status int, message string) {
	key := r.PathValue("authorKey")
	if !isOpenLibraryKey(key, 'A') {
		http.NotFound(w, r)
		return
	}

	author, err := h.queries.GetAuthorByOpenLibraryKey(r.Context(), text(key))
	if err == pgx.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
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
	books, err := h.queries.ListBooksByAuthor(r.Context(), store.ListBooksByAuthorParams{
		OpenLibraryAuthorKey: text(key),
		LibraryID:            libraryID,
	})
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	h.render(w, r, status, templates.AuthorPage(user, library, author, books, message))
}
