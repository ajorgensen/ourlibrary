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
	Title            string
	Subtitle         string
	WorkKey          string
	EditionKey       string
	ISBN             string
	CoverID          *int64
	FirstPublishYear *int32
	Authors          []authorSummary
}

type authorSummary struct {
	Key  string
	Name string
}

func (b bookMetadata) authorNames() string {
	names := make([]string, 0, len(b.Authors))
	for _, author := range b.Authors {
		names = append(names, author.Name)
	}

	return strings.Join(names, ", ")
}

type bookFinder interface {
	Find(context.Context, string) (bookMetadata, error)
}

type bookMetadataCache interface {
	FindByISBN(context.Context, string) (bookMetadata, error)
}

type localFirstBookFinder struct {
	cache    bookMetadataCache
	upstream bookFinder
}

func (f *localFirstBookFinder) Find(ctx context.Context, input string) (bookMetadata, error) {
	isbn, err := normalizeISBN(input)
	if err != nil {
		return bookMetadata{}, err
	}

	book, err := f.cache.FindByISBN(ctx, isbn)
	if err == nil {
		return book, nil
	}
	if !errors.Is(err, errBookNotFound) {
		return bookMetadata{}, fmt.Errorf("look up cached book: %w", err)
	}

	return f.upstream.Find(ctx, isbn)
}

type openLibraryBookFinder struct {
	client    *http.Client
	searchURL string
	userAgent string
}

func newOpenLibraryBookFinder(contact string) *openLibraryBookFinder {
	return &openLibraryBookFinder{
		client:    &http.Client{Timeout: 8 * time.Second},
		searchURL: openLibrarySearchURL,
		userAgent: openLibraryUserAgent(contact),
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
	query.Set("fields", "key,title,subtitle,author_name,author_key,cover_i,first_publish_year,editions,editions.key,editions.isbn")
	query.Set("limit", "1")
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return bookMetadata{}, fmt.Errorf("create Open Library request: %w", err)
	}
	if f.userAgent != "" {
		req.Header.Set("User-Agent", f.userAgent)
	}

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
			Key              string   `json:"key"`
			Title            string   `json:"title"`
			Subtitle         string   `json:"subtitle"`
			AuthorNames      []string `json:"author_name"`
			AuthorKeys       []string `json:"author_key"`
			CoverID          *int64   `json:"cover_i"`
			FirstPublishYear *int32   `json:"first_publish_year"`
			Editions         struct {
				Docs []struct {
					Key  string   `json:"key"`
					ISBN []string `json:"isbn"`
				} `json:"docs"`
			} `json:"editions"`
		} `json:"docs"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&result); err != nil {
		return bookMetadata{}, fmt.Errorf("decode Open Library response: %w", err)
	}

	if len(result.Docs) == 0 || strings.TrimSpace(result.Docs[0].Title) == "" {
		return bookMetadata{}, errBookNotFound
	}

	doc := result.Docs[0]
	authors := make([]authorSummary, 0, len(doc.AuthorNames))
	for index, name := range doc.AuthorNames {
		key := ""
		if index < len(doc.AuthorKeys) {
			key = trimOpenLibraryKey(doc.AuthorKeys[index])
		}
		authors = append(authors, authorSummary{Key: key, Name: strings.TrimSpace(name)})
	}

	editionKey := ""
	for _, edition := range doc.Editions.Docs {
		if containsISBN(edition.ISBN, isbn) {
			editionKey = trimOpenLibraryKey(edition.Key)
			break
		}
	}

	return bookMetadata{
		Title:            strings.TrimSpace(doc.Title),
		Subtitle:         strings.TrimSpace(doc.Subtitle),
		WorkKey:          trimOpenLibraryKey(doc.Key),
		EditionKey:       editionKey,
		ISBN:             isbn,
		CoverID:          doc.CoverID,
		FirstPublishYear: doc.FirstPublishYear,
		Authors:          authors,
	}, nil
}

func containsISBN(values []string, wanted string) bool {
	for _, value := range values {
		normalized, err := normalizeISBN(value)
		if err == nil && normalized == wanted {
			return true
		}
	}
	return false
}

func openLibraryUserAgent(contact string) string {
	contact = strings.TrimSpace(contact)
	if contact == "" {
		return "OurLibrary/0.1"
	}

	return "OurLibrary/0.1 (" + contact + ")"
}

func trimOpenLibraryKey(key string) string {
	key = strings.TrimSpace(key)
	key = strings.TrimPrefix(key, "/works/")
	key = strings.TrimPrefix(key, "/books/")
	key = strings.TrimPrefix(key, "/authors/")
	return key
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
