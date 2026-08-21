-- name: ListLibrariesForUser :many
SELECT libraries.id, libraries.name, libraries.created_at
FROM libraries
JOIN user_libraries ON user_libraries.library_id = libraries.id
WHERE user_libraries.user_id = $1
ORDER BY user_libraries.created_at, libraries.id;
