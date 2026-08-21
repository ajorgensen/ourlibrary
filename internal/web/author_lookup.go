package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const openLibraryBaseURL = "https://openlibrary.org"

type authorMetadata struct {
	Key       string
	Name      string
	Bio       string
	BirthDate string
	DeathDate string
}

type authorFinder interface {
	Find(context.Context, string) (authorMetadata, []bookMetadata, error)
}

type openLibraryAuthorFinder struct {
	client    *http.Client
	baseURL   string
	pageSize  int
	userAgent string
}

func newOpenLibraryAuthorFinder(contact string) *openLibraryAuthorFinder {
	return &openLibraryAuthorFinder{
		client:    &http.Client{Timeout: 15 * time.Second},
		baseURL:   openLibraryBaseURL,
		pageSize:  100,
		userAgent: openLibraryUserAgent(contact),
	}
}

func (f *openLibraryAuthorFinder) Find(ctx context.Context, key string) (authorMetadata, []bookMetadata, error) {
	key = trimOpenLibraryKey(key)
	if key == "" || strings.ContainsAny(key, "/?#") {
		return authorMetadata{}, nil, fmt.Errorf("invalid Open Library author key")
	}

	var detail struct {
		Name      string       `json:"name"`
		Bio       flexibleText `json:"bio"`
		BirthDate string       `json:"birth_date"`
		DeathDate string       `json:"death_date"`
	}
	if err := f.getJSON(ctx, f.baseURL+"/authors/"+url.PathEscape(key)+".json", &detail); err != nil {
		return authorMetadata{}, nil, fmt.Errorf("fetch author: %w", err)
	}

	author := authorMetadata{
		Key:       key,
		Name:      strings.TrimSpace(detail.Name),
		Bio:       strings.TrimSpace(string(detail.Bio)),
		BirthDate: strings.TrimSpace(detail.BirthDate),
		DeathDate: strings.TrimSpace(detail.DeathDate),
	}
	if author.Name == "" {
		return authorMetadata{}, nil, fmt.Errorf("fetch author: response has no name")
	}

	works, err := f.findWorks(ctx, author)
	if err != nil {
		return authorMetadata{}, nil, err
	}

	return author, works, nil
}

func (f *openLibraryAuthorFinder) findWorks(ctx context.Context, author authorMetadata) ([]bookMetadata, error) {
	pageSize := f.pageSize
	if pageSize <= 0 {
		pageSize = 100
	}

	works := make([]bookMetadata, 0)
	for offset := 0; ; offset += pageSize {
		endpoint := f.baseURL + "/authors/" + url.PathEscape(author.Key) + "/works.json?limit=" + strconv.Itoa(pageSize) + "&offset=" + strconv.Itoa(offset)
		var page struct {
			Size    int `json:"size"`
			Entries []struct {
				Key              string  `json:"key"`
				Title            string  `json:"title"`
				Subtitle         string  `json:"subtitle"`
				Covers           []int64 `json:"covers"`
				FirstPublishDate string  `json:"first_publish_date"`
			} `json:"entries"`
		}
		if err := f.getJSON(ctx, endpoint, &page); err != nil {
			return nil, fmt.Errorf("fetch author works: %w", err)
		}

		for _, entry := range page.Entries {
			work := bookMetadata{
				Title:            strings.TrimSpace(entry.Title),
				Subtitle:         strings.TrimSpace(entry.Subtitle),
				WorkKey:          trimOpenLibraryKey(entry.Key),
				FirstPublishYear: publicationYear(entry.FirstPublishDate),
				Authors:          []authorSummary{{Key: author.Key, Name: author.Name}},
			}
			if len(entry.Covers) > 0 && entry.Covers[0] > 0 {
				work.CoverID = &entry.Covers[0]
			}
			if work.Title != "" && work.WorkKey != "" {
				works = append(works, work)
			}
		}

		if len(page.Entries) == 0 || offset+len(page.Entries) >= page.Size {
			break
		}
	}

	return works, nil
}

func (f *openLibraryAuthorFinder) getJSON(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if f.userAgent != "" {
		req.Header.Set("User-Agent", f.userAgent)
	}

	response, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return fmt.Errorf("unexpected status %s", response.Status)
	}

	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}

type flexibleText string

func (t *flexibleText) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*t = flexibleText(text)
		return nil
	}

	var value struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*t = flexibleText(value.Value)
	return nil
}

func publicationYear(value string) *int32 {
	for _, field := range strings.FieldsFunc(value, func(r rune) bool { return r < '0' || r > '9' }) {
		if len(field) != 4 {
			continue
		}
		year, err := strconv.ParseInt(field, 10, 32)
		if err != nil || year < 1000 || year > 2999 {
			continue
		}
		parsed := int32(year)
		return &parsed
	}

	return nil
}
