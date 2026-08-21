package templates

import (
	"context"
	"strings"
	"testing"

	"github.com/ajorgensen/ourlibrary/internal/database/store"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestBookListLinksToBookPage(t *testing.T) {
	id := pgtype.UUID{Bytes: [16]byte{15: 1}, Valid: true}
	var output strings.Builder
	if err := BookList([]store.ListBooksRow{{ID: id, Title: "Fantastic Mr Fox"}}).Render(context.Background(), &output); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// This path is the navigation contract between a catalog card and its detail page.
	if !strings.Contains(output.String(), `href="/books/00000000-0000-0000-0000-000000000001"`) {
		t.Errorf("book list did not link to the local book page: %s", output.String())
	}
}

func TestBookPageRendersStoredCatalogMetadata(t *testing.T) {
	book := store.GetBookRow{
		Title:              "Fantastic Mr. Fox",
		FirstPublishYear:   pgtype.Int4{Int32: 1970, Valid: true},
		OpenLibraryWorkKey: pgtype.Text{String: "OL45804W", Valid: true},
		InLibrary:          true,
		AuthorNames:        []string{"Roald Dahl"},
		AuthorKeys:         []string{"OL34184A"},
		Isbns:              []string{"9780140328721"},
	}
	library := &store.Library{Name: "Local Library"}
	var output strings.Builder
	if err := BookPage(store.User{}, library, book).Render(context.Background(), &output); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := output.String()
	for _, expected := range []string{"Fantastic Mr. Fox", "Roald Dahl", "9780140328721", "In Local Library", "View on Open Library"} {
		if !strings.Contains(body, expected) {
			t.Errorf("book page did not render %q: %s", expected, body)
		}
	}
}

func TestBookPageRendersManualDiscoveredBookWithoutLibrary(t *testing.T) {
	book := store.GetBookRow{Title: "A Manual Book", AuthorNames: []string{"A Local Author"}}
	var output strings.Builder
	if err := BookPage(store.User{}, nil, book).Render(context.Background(), &output); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := output.String()
	if !strings.Contains(body, "A Manual Book") || !strings.Contains(body, "A Local Author") || !strings.Contains(body, "Discovered book") {
		t.Errorf("manual book page did not render its local metadata: %s", body)
	}
}
