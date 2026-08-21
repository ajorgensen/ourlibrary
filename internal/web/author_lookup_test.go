package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenLibraryAuthorFinderFetchesMetadataAndAllWorkPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/authors/OL34184A.json":
			_, _ = io.WriteString(w, `{"name":"Roald Dahl","bio":{"value":"British author"},"birth_date":"13 September 1916","death_date":"23 November 1990"}`)
		case "/authors/OL34184A/works.json":
			if r.URL.Query().Get("offset") == "0" {
				_, _ = io.WriteString(w, `{"size":2,"entries":[{"key":"/works/OL45804W","title":"Fantastic Mr Fox","covers":[8739161],"first_publish_date":"1970"}]}`)
				return
			}
			_, _ = io.WriteString(w, `{"size":2,"entries":[{"key":"/works/OL17798712W","title":"The Minpins","first_publish_date":"1991"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	finder := &openLibraryAuthorFinder{client: server.Client(), baseURL: server.URL, pageSize: 1}
	author, works, err := finder.Find(context.Background(), "OL34184A")
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if author.Name != "Roald Dahl" || author.Bio != "British author" {
		t.Errorf("Author = %#v, want decoded author metadata", author)
	}
	if len(works) != 2 || works[0].WorkKey != "OL45804W" || works[1].Title != "The Minpins" {
		t.Errorf("Works = %#v, want both pages", works)
	}
	if works[0].FirstPublishYear == nil || *works[0].FirstPublishYear != 1970 {
		t.Errorf("FirstPublishYear = %v, want parsed year", works[0].FirstPublishYear)
	}
}
