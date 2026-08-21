package web

import (
	"context"
	"errors"
	"fmt"

	"github.com/ajorgensen/ourlibrary/internal/database/store"
	"github.com/jackc/pgx/v5"
)

type databaseBookMetadataCache struct {
	queries bookMetadataQueries
}

type bookMetadataQueries interface {
	FindBookMetadataByISBN(context.Context, string) (store.FindBookMetadataByISBNRow, error)
}

func (c *databaseBookMetadataCache) FindByISBN(ctx context.Context, isbn string) (bookMetadata, error) {
	row, err := c.queries.FindBookMetadataByISBN(ctx, isbn)
	if errors.Is(err, pgx.ErrNoRows) {
		return bookMetadata{}, errBookNotFound
	}
	if err != nil {
		return bookMetadata{}, fmt.Errorf("query book by ISBN: %w", err)
	}

	authors := make([]authorSummary, 0, len(row.AuthorNames))
	for index, name := range row.AuthorNames {
		key := ""
		if index < len(row.AuthorKeys) {
			key = row.AuthorKeys[index]
		}
		authors = append(authors, authorSummary{Key: key, Name: name})
	}

	var coverID *int64
	if row.CoverID.Valid {
		coverID = &row.CoverID.Int64
	}
	var firstPublishYear *int32
	if row.FirstPublishYear.Valid {
		firstPublishYear = &row.FirstPublishYear.Int32
	}

	return bookMetadata{
		Title:            row.Title,
		Subtitle:         row.Subtitle.String,
		WorkKey:          row.OpenLibraryWorkKey.String,
		EditionKey:       row.OpenLibraryEditionKey,
		ISBN:             isbn,
		CoverID:          coverID,
		FirstPublishYear: firstPublishYear,
		Authors:          authors,
	}, nil
}
