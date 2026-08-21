package web

import (
	"context"
	"errors"
	"testing"

	"github.com/ajorgensen/ourlibrary/internal/database/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestDatabaseBookMetadataCacheMapsCanonicalMetadata(t *testing.T) {
	cache := &databaseBookMetadataCache{queries: bookMetadataQueriesFunc(func(ctx context.Context, isbn string) (store.FindBookMetadataByISBNRow, error) {
		return store.FindBookMetadataByISBNRow{
			Title:                 "Fantastic Mr Fox",
			Subtitle:              pgtype.Text{String: "A novel", Valid: true},
			CoverID:               pgtype.Int8{Int64: 8739161, Valid: true},
			FirstPublishYear:      pgtype.Int4{Int32: 1970, Valid: true},
			OpenLibraryWorkKey:    pgtype.Text{String: "OL45804W", Valid: true},
			OpenLibraryEditionKey: "OL7353617M",
			AuthorNames:           []string{"Roald Dahl", "Quentin Blake"},
			AuthorKeys:            []string{"OL34184A", "OL28285A"},
		}, nil
	})}

	book, err := cache.FindByISBN(context.Background(), "9780140328721")
	if err != nil {
		t.Fatalf("FindByISBN() error = %v", err)
	}
	if book.Title != "Fantastic Mr Fox" || book.Subtitle != "A novel" {
		t.Errorf("book text metadata = %#v", book)
	}
	if book.WorkKey != "OL45804W" || book.EditionKey != "OL7353617M" || book.ISBN != "9780140328721" {
		t.Errorf("book identifiers = %#v", book)
	}
	if book.CoverID == nil || *book.CoverID != 8739161 || book.FirstPublishYear == nil || *book.FirstPublishYear != 1970 {
		t.Errorf("book presentation metadata = %#v", book)
	}
	if len(book.Authors) != 2 || book.Authors[0].Key != "OL34184A" || book.Authors[1].Name != "Quentin Blake" {
		t.Errorf("book authors = %#v", book.Authors)
	}
}

func TestDatabaseBookMetadataCacheMapsNoRowsToCacheMiss(t *testing.T) {
	cache := &databaseBookMetadataCache{queries: bookMetadataQueriesFunc(func(ctx context.Context, isbn string) (store.FindBookMetadataByISBNRow, error) {
		return store.FindBookMetadataByISBNRow{}, pgx.ErrNoRows
	})}

	_, err := cache.FindByISBN(context.Background(), "9780140328721")
	if !errors.Is(err, errBookNotFound) {
		t.Fatalf("FindByISBN() error = %v, want book not found", err)
	}
}

func TestDatabaseBookMetadataCacheReturnsDatabaseFailure(t *testing.T) {
	wanted := errors.New("database unavailable")
	cache := &databaseBookMetadataCache{queries: bookMetadataQueriesFunc(func(ctx context.Context, isbn string) (store.FindBookMetadataByISBNRow, error) {
		return store.FindBookMetadataByISBNRow{}, wanted
	})}

	_, err := cache.FindByISBN(context.Background(), "9780140328721")
	if !errors.Is(err, wanted) {
		t.Fatalf("FindByISBN() error = %v, want database failure", err)
	}
}

type bookMetadataQueriesFunc func(context.Context, string) (store.FindBookMetadataByISBNRow, error)

func (f bookMetadataQueriesFunc) FindBookMetadataByISBN(ctx context.Context, isbn string) (store.FindBookMetadataByISBNRow, error) {
	return f(ctx, isbn)
}
