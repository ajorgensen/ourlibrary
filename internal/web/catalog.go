package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ajorgensen/ourlibrary/internal/database/store"
	"github.com/jackc/pgx/v5/pgtype"
)

func bookMetadataFromForm(r *http.Request, title string) bookMetadata {
	workKey := trimOpenLibraryKey(r.FormValue("work_key"))
	if !isOpenLibraryKey(workKey, 'W') {
		return bookMetadata{Title: title}
	}
	isbn := matchingLookupISBN(r.FormValue("isbn"), r.FormValue("lookup_isbn"))
	if isbn == "" {
		return bookMetadata{Title: title}
	}

	var authors []authorSummary
	if err := json.Unmarshal([]byte(r.FormValue("authors_json")), &authors); err != nil || len(authors) == 0 {
		return bookMetadata{Title: title}
	}
	for _, item := range authors {
		if !isOpenLibraryKey(item.Key, 'A') || strings.TrimSpace(item.Name) == "" {
			return bookMetadata{Title: title}
		}
	}
	lookedUpTitle := strings.TrimSpace(r.FormValue("looked_up_title"))
	if lookedUpTitle == "" {
		return bookMetadata{Title: title}
	}

	editionKey := trimOpenLibraryKey(r.FormValue("edition_key"))
	if !isOpenLibraryKey(editionKey, 'M') {
		editionKey = ""
	}
	coverID := positiveInt64(r.FormValue("cover_id"))
	firstPublishYear := int32Value(r.FormValue("first_publish_year"))

	return bookMetadata{
		Title:            lookedUpTitle,
		Subtitle:         strings.TrimSpace(r.FormValue("subtitle")),
		WorkKey:          workKey,
		EditionKey:       editionKey,
		ISBN:             isbn,
		CoverID:          coverID,
		FirstPublishYear: firstPublishYear,
		Authors:          authors,
	}
}

func (h *handler) addBook(ctx context.Context, metadata bookMetadata, displayTitle, displayAuthor string, libraryID, userID pgtype.UUID) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin add book: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := h.queries.WithTx(tx)

	book, err := createCatalogBook(ctx, queries, metadata, displayAuthor)
	if err != nil {
		return err
	}

	if err := queries.AddBookToLibrary(ctx, store.AddBookToLibraryParams{
		LibraryID:      libraryID,
		BookID:         book.ID,
		AddedByUserID:  userID,
		TitleOverride:  override(displayTitle, metadata.Title, metadata.WorkKey != ""),
		AuthorOverride: override(displayAuthor, metadata.authorNames(), metadata.WorkKey != ""),
	}); err != nil {
		return fmt.Errorf("add book to library: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit add book: %w", err)
	}

	return nil
}

func createCatalogBook(ctx context.Context, queries *store.Queries, metadata bookMetadata, manualAuthor string) (store.Book, error) {
	if metadata.WorkKey == "" {
		return createManualBook(ctx, queries, metadata.Title, manualAuthor)
	}

	book, err := upsertOpenLibraryBook(ctx, queries, metadata)
	if err != nil {
		return store.Book{}, err
	}
	for position, item := range metadata.Authors {
		author, upsertErr := upsertAuthor(ctx, queries, item)
		if upsertErr != nil {
			return store.Book{}, fmt.Errorf("upsert author: %w", upsertErr)
		}
		if err := queries.LinkBookAuthor(ctx, store.LinkBookAuthorParams{BookID: book.ID, AuthorID: author.ID, Position: int32(position)}); err != nil {
			return store.Book{}, fmt.Errorf("link author: %w", err)
		}
	}
	if metadata.ISBN != "" {
		if err := queries.AddBookIdentifier(ctx, store.AddBookIdentifierParams{Kind: "isbn", Value: metadata.ISBN, BookID: book.ID}); err != nil {
			return store.Book{}, fmt.Errorf("save ISBN: %w", err)
		}
	}
	if metadata.EditionKey != "" {
		if err := queries.AddBookIdentifier(ctx, store.AddBookIdentifierParams{Kind: "open_library_edition", Value: metadata.EditionKey, BookID: book.ID}); err != nil {
			return store.Book{}, fmt.Errorf("save edition key: %w", err)
		}
	}

	return book, nil
}

func upsertAuthor(ctx context.Context, queries *store.Queries, metadata authorSummary) (store.Author, error) {
	if metadata.Key == "" {
		return queries.CreateManualAuthor(ctx, metadata.Name)
	}
	return queries.UpsertOpenLibraryAuthor(ctx, store.UpsertOpenLibraryAuthorParams{
		OpenLibraryAuthorKey: text(metadata.Key),
		Name:                 metadata.Name,
	})
}

func override(display, canonical string, enabled bool) pgtype.Text {
	display = strings.TrimSpace(display)
	if !enabled || display == strings.TrimSpace(canonical) {
		return pgtype.Text{}
	}
	return text(display)
}

func createManualBook(ctx context.Context, queries *store.Queries, title, authorName string) (store.Book, error) {
	book, err := queries.CreateManualBook(ctx, title)
	if err != nil {
		return store.Book{}, fmt.Errorf("create manual book: %w", err)
	}
	author, err := queries.CreateManualAuthor(ctx, authorName)
	if err != nil {
		return store.Book{}, fmt.Errorf("create manual author: %w", err)
	}
	if err := queries.LinkBookAuthor(ctx, store.LinkBookAuthorParams{BookID: book.ID, AuthorID: author.ID}); err != nil {
		return store.Book{}, fmt.Errorf("link manual author: %w", err)
	}

	return book, nil
}

func matchingLookupISBN(current, lookedUp string) string {
	currentISBN, currentErr := normalizeISBN(current)
	lookedUpISBN, lookedUpErr := normalizeISBN(lookedUp)
	if currentErr != nil || lookedUpErr != nil || currentISBN != lookedUpISBN {
		return ""
	}
	return lookedUpISBN
}

func positiveInt64(value string) *int64 {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return nil
	}
	return &parsed
}

func int32Value(value string) *int32 {
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return nil
	}
	result := int32(parsed)
	return &result
}

func upsertOpenLibraryBook(ctx context.Context, queries *store.Queries, metadata bookMetadata) (store.Book, error) {
	book, err := queries.UpsertOpenLibraryBook(ctx, store.UpsertOpenLibraryBookParams{
		OpenLibraryWorkKey: text(metadata.WorkKey),
		Title:              metadata.Title,
		Subtitle:           text(metadata.Subtitle),
		CoverID:            int8(metadata.CoverID),
		FirstPublishYear:   int4(metadata.FirstPublishYear),
	})
	if err != nil {
		return store.Book{}, fmt.Errorf("upsert Open Library book: %w", err)
	}
	return book, nil
}

func isOpenLibraryKey(key string, suffix byte) bool {
	if len(key) < 4 || !strings.HasPrefix(key, "OL") || key[len(key)-1] != suffix {
		return false
	}
	for _, character := range key[2 : len(key)-1] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func text(value string) pgtype.Text {
	value = strings.TrimSpace(value)
	return pgtype.Text{String: value, Valid: value != ""}
}

func int8(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func int4(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}

func formatInt64(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}

func formatInt32(value *int32) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(int64(*value), 10)
}
