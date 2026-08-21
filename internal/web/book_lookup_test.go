package web

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOpenLibraryBookFinderFindsBook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The isbn parameter is the external API contract that selects the requested book.
		if got := r.URL.Query().Get("isbn"); got != "9780140328721" {
			t.Errorf("ISBN query = %q, want normalized ISBN", got)
		}

		_, _ = io.WriteString(w, `{"docs":[{
			"key":"/works/OL45804W",
			"title":"Fantastic Mr Fox",
			"author_name":["Roald Dahl","Quentin Blake"],
			"author_key":["OL34184A","OL28285A"],
			"cover_i":8739161,
			"first_publish_year":1970,
			"editions":{"docs":[
				{"key":"/books/OL111M","isbn":["9780000000002"]},
				{"key":"/books/OL7353617M","isbn":["9780140328721"]}
			]}
		}]}`)
	}))
	t.Cleanup(server.Close)

	finder := &openLibraryBookFinder{client: server.Client(), searchURL: server.URL}
	book, err := finder.Find(context.Background(), "978-0-14-032872-1")
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}

	if book.Title != "Fantastic Mr Fox" {
		t.Errorf("Title = %q, want %q", book.Title, "Fantastic Mr Fox")
	}
	if book.WorkKey != "OL45804W" || book.EditionKey != "OL7353617M" {
		t.Errorf("Open Library keys = %q/%q, want work and edition keys", book.WorkKey, book.EditionKey)
	}
	if book.ISBN != "9780140328721" {
		t.Errorf("ISBN = %q, want normalized ISBN", book.ISBN)
	}
	if len(book.Authors) != 2 || book.Authors[0].Key != "OL34184A" || book.Authors[1].Name != "Quentin Blake" {
		t.Errorf("Authors = %#v, want keyed authors in response order", book.Authors)
	}
	if book.CoverID == nil || *book.CoverID != 8739161 {
		t.Errorf("CoverID = %v, want response cover", book.CoverID)
	}
}

func TestOpenLibraryBookFinderRejectsInvalidISBNBeforeRequest(t *testing.T) {
	requested := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = true
	}))
	t.Cleanup(server.Close)

	finder := &openLibraryBookFinder{client: server.Client(), searchURL: server.URL}
	_, err := finder.Find(context.Background(), "9780140328722")
	if !errors.Is(err, errInvalidISBN) {
		t.Fatalf("Find() error = %v, want invalid ISBN", err)
	}
	if requested {
		t.Error("Find() made a request for an invalid ISBN")
	}
}

func TestOpenLibraryBookFinderReturnsNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"docs":[]}`)
	}))
	t.Cleanup(server.Close)

	finder := &openLibraryBookFinder{client: server.Client(), searchURL: server.URL}
	_, err := finder.Find(context.Background(), "9780140328721")
	if !errors.Is(err, errBookNotFound) {
		t.Fatalf("Find() error = %v, want book not found", err)
	}
}

func TestLookupBookFillsEditableFields(t *testing.T) {
	h := lookupTestHandler(bookFinderFunc(func(ctx context.Context, isbn string) (bookMetadata, error) {
		return bookMetadata{
			Title:      "Fantastic Mr Fox",
			WorkKey:    "OL45804W",
			EditionKey: "OL7353617M",
			ISBN:       "9780140328721",
			Authors:    []authorSummary{{Key: "OL34184A", Name: "Roald Dahl"}},
		}, nil
	}))
	request := httptest.NewRequest(http.MethodGet, "/books/lookup?isbn=9780140328721", nil)
	response := httptest.NewRecorder()

	h.lookupBook(response, request)

	body := response.Body.String()
	if !strings.Contains(body, `name="title" value="Fantastic Mr Fox"`) {
		t.Errorf("response did not fill the title field: %s", body)
	}
	if !strings.Contains(body, `name="author" value="Roald Dahl"`) {
		t.Errorf("response did not fill the author field: %s", body)
	}
	if !strings.Contains(body, `name="work_key" value="OL45804W"`) {
		t.Errorf("response did not retain the work identity: %s", body)
	}
	if !strings.Contains(body, "Review or edit the details") {
		t.Errorf("response did not ask the user to review the book: %s", body)
	}
}

func TestLookupBookPreservesManualFieldsWhenLookupFails(t *testing.T) {
	h := lookupTestHandler(bookFinderFunc(func(ctx context.Context, isbn string) (bookMetadata, error) {
		return bookMetadata{}, errBookNotFound
	}))
	query := make(url.Values)
	query.Set("isbn", "9780140328721")
	query.Set("title", "My title")
	query.Set("author", "My author")
	request := httptest.NewRequest(http.MethodGet, "/books/lookup?"+query.Encode(), nil)
	response := httptest.NewRecorder()

	h.lookupBook(response, request)

	body := response.Body.String()
	if !strings.Contains(body, `name="title" value="My title"`) {
		t.Errorf("response did not preserve the title field: %s", body)
	}
	if !strings.Contains(body, `name="author" value="My author"`) {
		t.Errorf("response did not preserve the author field: %s", body)
	}
	if !strings.Contains(body, "enter the details manually") {
		t.Errorf("response did not explain the lookup fallback: %s", body)
	}
}

type bookFinderFunc func(context.Context, string) (bookMetadata, error)

func (f bookFinderFunc) Find(ctx context.Context, isbn string) (bookMetadata, error) {
	return f(ctx, isbn)
}

func lookupTestHandler(finder bookFinder) *handler {
	return &handler{
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		bookFinder: finder,
	}
}
