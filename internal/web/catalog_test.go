package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestBookMetadataFromFormKeepsIdentityWhenAuthorIsEdited(t *testing.T) {
	form := canonicalBookForm()
	form.Set("author", "Roald Dahl, Sir Quentin Blake")
	request := httptest.NewRequest(http.MethodPost, "/books", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	metadata := bookMetadataFromForm(request, form.Get("title"))

	if metadata.WorkKey == "" || metadata.ISBN == "" {
		t.Fatalf("metadata lost canonical identity after an author edit: %#v", metadata)
	}
	if len(metadata.Authors) != 2 || metadata.Authors[1].Name != "Quentin Blake" || metadata.Authors[1].Key == "" {
		t.Errorf("Authors = %#v, want canonical authors to remain shared", metadata.Authors)
	}
	if got := override(form.Get("author"), metadata.authorNames(), true); !got.Valid || got.String != "Roald Dahl, Sir Quentin Blake" {
		t.Errorf("author override = %#v, want the library-specific edit", got)
	}
}

func TestBookMetadataFromFormRejectsStaleLookupIdentity(t *testing.T) {
	form := canonicalBookForm()
	form.Set("isbn", "9780306406157")
	request := httptest.NewRequest(http.MethodPost, "/books", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	metadata := bookMetadataFromForm(request, form.Get("title"))

	if metadata.WorkKey != "" || metadata.ISBN != "" {
		t.Errorf("metadata retained identity from a different ISBN lookup: %#v", metadata)
	}
}

func canonicalBookForm() url.Values {
	form := make(url.Values)
	form.Set("isbn", "9780140328721")
	form.Set("lookup_isbn", "9780140328721")
	form.Set("title", "Fantastic Mr Fox")
	form.Set("author", "Roald Dahl, Quentin Blake")
	form.Set("looked_up_author", "Roald Dahl, Quentin Blake")
	form.Set("looked_up_title", "Fantastic Mr Fox")
	form.Set("work_key", "OL45804W")
	form.Set("edition_key", "OL7353617M")
	form.Set("authors_json", `[{"Key":"OL34184A","Name":"Roald Dahl"},{"Key":"OL28285A","Name":"Quentin Blake"}]`)
	return form
}
