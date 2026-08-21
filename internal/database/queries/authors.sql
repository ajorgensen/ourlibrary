-- name: GetAuthorByOpenLibraryKey :one
SELECT *
FROM authors
WHERE open_library_author_key = $1;

-- name: UpdateAuthorMetadata :one
UPDATE authors
SET
    name = $2,
    bio = $3,
    birth_date = $4,
    death_date = $5,
    metadata_synced_at = now()
WHERE open_library_author_key = $1
RETURNING *;

-- name: MarkAuthorWorksSynced :exec
UPDATE authors
SET works_synced_at = now()
WHERE open_library_author_key = $1;

-- name: AddAuthorBookToLibrary :exec
INSERT INTO library_books (library_id, book_id, added_by_user_id)
SELECT sqlc.arg(library_id), books.id, sqlc.arg(added_by_user_id)
FROM books
JOIN book_authors ON book_authors.book_id = books.id
JOIN authors ON authors.id = book_authors.author_id
WHERE authors.open_library_author_key = sqlc.arg(open_library_author_key)
  AND books.id = sqlc.arg(book_id)
ON CONFLICT (library_id, book_id) DO NOTHING;

-- name: ListBooksByAuthor :many
SELECT
    books.id,
    COALESCE(library_books.title_override, books.title)::text AS title,
    books.subtitle,
    books.cover_id,
    books.first_publish_year,
    CASE WHEN library_books.book_id IS NULL THEN false ELSE true END AS in_library
FROM books
JOIN book_authors ON book_authors.book_id = books.id
JOIN authors ON authors.id = book_authors.author_id
LEFT JOIN library_books
    ON library_books.book_id = books.id
   AND library_books.library_id = $2
WHERE authors.open_library_author_key = $1
ORDER BY books.first_publish_year NULLS LAST, books.title, books.id;
