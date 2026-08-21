package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const openLibrarySearchURL = "https://openlibrary.org/search.json"

var (
	errInvalidISBN  = errors.New("invalid ISBN")
	errBookNotFound = errors.New("book not found")
)

type bookMetadata struct {
	Title  string
	Author string
}

type bookFinder interface {
	Find(context.Context, string) (bookMetadata, error)
}

type openLibraryBookFinder struct {
	client    *http.Client
	searchURL string
}

func newOpenLibraryBookFinder() *openLibraryBookFinder {
	return &openLibraryBookFinder{
		client:    &http.Client{Timeout: 8 * time.Second},
		searchURL: openLibrarySearchURL,
	}
}

func (f *openLibraryBookFinder) Find(ctx context.Context, input string) (bookMetadata, error) {
	isbn, err := normalizeISBN(input)
	if err != nil {
		return bookMetadata{}, err
	}

	endpoint, err := url.Parse(f.searchURL)
	if err != nil {
		return bookMetadata{}, fmt.Errorf("parse Open Library URL: %w", err)
	}

	query := endpoint.Query()
	query.Set("isbn", isbn)
	query.Set("fields", "title,author_name")
	query.Set("limit", "1")
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return bookMetadata{}, fmt.Errorf("create Open Library request: %w", err)
	}
	req.Header.Set("User-Agent", "OurLibrary/0.1")

	response, err := f.client.Do(req)
	if err != nil {
		return bookMetadata{}, fmt.Errorf("query Open Library: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return bookMetadata{}, fmt.Errorf("query Open Library: unexpected status %s", response.Status)
	}

	var result struct {
		Docs []struct {
			Title   string   `json:"title"`
			Authors []string `json:"author_name"`
		} `json:"docs"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&result); err != nil {
		return bookMetadata{}, fmt.Errorf("decode Open Library response: %w", err)
	}

	if len(result.Docs) == 0 || strings.TrimSpace(result.Docs[0].Title) == "" {
		return bookMetadata{}, errBookNotFound
	}

	return bookMetadata{
		Title:  strings.TrimSpace(result.Docs[0].Title),
		Author: strings.Join(result.Docs[0].Authors, ", "),
	}, nil
}

func normalizeISBN(input string) (string, error) {
	var normalized strings.Builder
	for _, character := range strings.TrimSpace(input) {
		switch {
		case character >= '0' && character <= '9':
			normalized.WriteRune(character)
		case character == 'X' || character == 'x':
			normalized.WriteByte('X')
		case character == '-' || character == ' ' || character == '\t':
			continue
		default:
			return "", errInvalidISBN
		}
	}

	isbn := normalized.String()
	if isValidISBN10(isbn) || isValidISBN13(isbn) {
		return isbn, nil
	}

	return "", errInvalidISBN
}

func isValidISBN10(isbn string) bool {
	if len(isbn) != 10 {
		return false
	}

	sum := 0
	for index := 0; index < 9; index++ {
		if isbn[index] < '0' || isbn[index] > '9' {
			return false
		}
		sum += int(isbn[index]-'0') * (10 - index)
	}

	checkDigit := 10
	if isbn[9] >= '0' && isbn[9] <= '9' {
		checkDigit = int(isbn[9] - '0')
	} else if isbn[9] != 'X' {
		return false
	}

	return (sum+checkDigit)%11 == 0
}

func isValidISBN13(isbn string) bool {
	if len(isbn) != 13 || (!strings.HasPrefix(isbn, "978") && !strings.HasPrefix(isbn, "979")) {
		return false
	}

	sum := 0
	for index := 0; index < 12; index++ {
		if isbn[index] < '0' || isbn[index] > '9' {
			return false
		}
		weight := 1
		if index%2 == 1 {
			weight = 3
		}
		sum += int(isbn[index]-'0') * weight
	}

	if isbn[12] < '0' || isbn[12] > '9' {
		return false
	}

	return (10-(sum%10))%10 == int(isbn[12]-'0')
}
