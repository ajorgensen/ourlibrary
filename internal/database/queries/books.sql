-- name: ListBooks :many
SELECT
    books.id,
    COALESCE(library_books.title_override, books.title)::text AS title,
    books.subtitle,
    books.cover_id,
    books.first_publish_year,
    library_books.added_at,
    library_books.author_override,
    COALESCE(
        array_agg(authors.name ORDER BY book_authors.position, authors.id)
            FILTER (WHERE authors.id IS NOT NULL),
        ARRAY[]::text[]
    )::text[] AS author_names_list,
    COALESCE(
        array_agg(COALESCE(authors.open_library_author_key, '') ORDER BY book_authors.position, authors.id)
            FILTER (WHERE authors.id IS NOT NULL),
        ARRAY[]::text[]
    )::text[] AS author_keys
FROM library_books
JOIN books ON books.id = library_books.book_id
LEFT JOIN book_authors ON book_authors.book_id = books.id
LEFT JOIN authors ON authors.id = book_authors.author_id
WHERE library_books.library_id = $1
GROUP BY books.id, library_books.added_at, library_books.title_override, library_books.author_override
ORDER BY library_books.added_at DESC, books.id DESC;

-- name: GetBook :one
SELECT
    books.id,
    COALESCE(library_books.title_override, books.title)::text AS title,
    books.subtitle,
    books.cover_id,
    books.first_publish_year,
    books.open_library_work_key,
    CASE WHEN library_books.book_id IS NULL THEN false ELSE true END AS in_library,
    library_books.author_override,
    COALESCE(
        (SELECT array_agg(authors.name ORDER BY book_authors.position, authors.id)
         FROM book_authors
         JOIN authors ON authors.id = book_authors.author_id
         WHERE book_authors.book_id = books.id),
        ARRAY[]::text[]
    )::text[] AS author_names,
    COALESCE(
        (SELECT array_agg(COALESCE(authors.open_library_author_key, '') ORDER BY book_authors.position, authors.id)
         FROM book_authors
         JOIN authors ON authors.id = book_authors.author_id
         WHERE book_authors.book_id = books.id),
        ARRAY[]::text[]
    )::text[] AS author_keys,
    COALESCE(
        (SELECT array_agg(book_identifiers.value ORDER BY book_identifiers.value)
         FROM book_identifiers
         WHERE book_identifiers.book_id = books.id
           AND book_identifiers.kind = 'isbn'),
        ARRAY[]::text[]
    )::text[] AS isbns
FROM books
LEFT JOIN library_books
    ON library_books.book_id = books.id
   AND library_books.library_id = $2
WHERE books.id = $1;

-- name: FindBookMetadataByISBN :one
SELECT
    books.title,
    books.subtitle,
    books.cover_id,
    books.first_publish_year,
    books.open_library_work_key,
    COALESCE(
        (SELECT min(edition_identifiers.value)
         FROM book_identifiers AS edition_identifiers
         WHERE edition_identifiers.book_id = books.id
           AND edition_identifiers.kind = 'open_library_edition'
         HAVING count(*) = 1
            AND (SELECT count(*)
                 FROM book_identifiers AS isbn_identifiers
                 WHERE isbn_identifiers.book_id = books.id
                   AND isbn_identifiers.kind = 'isbn') = 1),
        ''
    )::text AS open_library_edition_key,
    COALESCE(
        (SELECT array_agg(authors.name ORDER BY book_authors.position, authors.id)
         FROM book_authors
         JOIN authors ON authors.id = book_authors.author_id
         WHERE book_authors.book_id = books.id),
        ARRAY[]::text[]
    )::text[] AS author_names,
    COALESCE(
        (SELECT array_agg(COALESCE(authors.open_library_author_key, '') ORDER BY book_authors.position, authors.id)
         FROM book_authors
         JOIN authors ON authors.id = book_authors.author_id
         WHERE book_authors.book_id = books.id),
        ARRAY[]::text[]
    )::text[] AS author_keys
FROM book_identifiers
JOIN books ON books.id = book_identifiers.book_id
WHERE book_identifiers.kind = 'isbn'
  AND book_identifiers.value = $1;

-- name: CreateManualBook :one
INSERT INTO books (title)
VALUES ($1)
RETURNING *;

-- name: UpsertOpenLibraryBook :one
INSERT INTO books (
    open_library_work_key,
    title,
    subtitle,
    cover_id,
    first_publish_year,
    metadata_updated_at
)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (open_library_work_key) DO UPDATE SET
    title = EXCLUDED.title,
    subtitle = EXCLUDED.subtitle,
    cover_id = COALESCE(EXCLUDED.cover_id, books.cover_id),
    first_publish_year = COALESCE(EXCLUDED.first_publish_year, books.first_publish_year),
    metadata_updated_at = now()
RETURNING *;

-- name: AddBookIdentifier :exec
INSERT INTO book_identifiers (kind, value, book_id)
VALUES ($1, $2, $3)
ON CONFLICT (kind, value) DO UPDATE SET book_id = EXCLUDED.book_id;

-- name: AddBookToLibrary :exec
INSERT INTO library_books (library_id, book_id, added_by_user_id, title_override, author_override)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (library_id, book_id) DO UPDATE SET
    title_override = EXCLUDED.title_override,
    author_override = EXCLUDED.author_override;

-- name: CreateManualAuthor :one
INSERT INTO authors (name)
VALUES ($1)
RETURNING *;

-- name: UpsertOpenLibraryAuthor :one
INSERT INTO authors (open_library_author_key, name)
VALUES ($1, $2)
ON CONFLICT (open_library_author_key) DO UPDATE SET name = EXCLUDED.name
RETURNING *;

-- name: LinkBookAuthor :exec
INSERT INTO book_authors (book_id, author_id, position)
VALUES ($1, $2, $3)
ON CONFLICT (book_id, author_id) DO UPDATE SET position = EXCLUDED.position;
