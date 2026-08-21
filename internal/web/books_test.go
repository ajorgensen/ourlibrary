package web

import "testing"

func TestParseUUID(t *testing.T) {
	id, ok := parseUUID("54289c53-dd01-44a0-91e8-74d60376b3ab")
	if !ok || !id.Valid {
		t.Fatal("parseUUID() rejected a valid book ID")
	}

	if _, ok := parseUUID("not-a-book-id"); ok {
		t.Fatal("parseUUID() accepted an invalid book ID")
	}
}
