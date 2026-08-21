-- +goose Up
-- +goose StatementBegin
DO $$
DECLARE
    default_user_id UUID;
    default_library_id UUID;
BEGIN
    INSERT INTO users (email)
    VALUES ('dev@example.com')
    ON CONFLICT DO NOTHING;

    SELECT id INTO default_user_id
    FROM users
    WHERE email = 'dev@example.com';

    SELECT libraries.id INTO default_library_id
    FROM libraries
    JOIN user_libraries ON user_libraries.library_id = libraries.id
    WHERE user_libraries.user_id = default_user_id
      AND libraries.name = 'Local Library'
    LIMIT 1;

    IF default_library_id IS NULL THEN
        INSERT INTO libraries (name)
        VALUES ('Local Library')
        RETURNING id INTO default_library_id;
    END IF;

    INSERT INTO user_libraries (user_id, library_id)
    VALUES (default_user_id, default_library_id)
    ON CONFLICT DO NOTHING;
END $$;
-- +goose StatementEnd
