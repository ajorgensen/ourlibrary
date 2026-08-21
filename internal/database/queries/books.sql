-- name: ListBooks :many
SELECT id, title, author, created_at
FROM books
ORDER BY created_at DESC, id DESC;

-- name: CreateBook :one
INSERT INTO books (title, author)
VALUES ($1, $2)
RETURNING id, title, author, created_at;
