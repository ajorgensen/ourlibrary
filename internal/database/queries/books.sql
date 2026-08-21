-- name: ListBooks :many
SELECT id, title, author, created_at, library_id
FROM books
WHERE library_id = $1
ORDER BY created_at DESC, id DESC;

-- name: CreateBook :one
INSERT INTO books (library_id, title, author)
VALUES ($1, $2, $3)
RETURNING id, title, author, created_at, library_id;
