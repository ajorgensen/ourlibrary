-- +goose Up
ALTER TABLE books
    ADD COLUMN open_library_work_key TEXT UNIQUE,
    ADD COLUMN subtitle TEXT,
    ADD COLUMN cover_id BIGINT,
    ADD COLUMN first_publish_year INTEGER,
    ADD COLUMN metadata_updated_at TIMESTAMPTZ;

CREATE TABLE authors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    open_library_author_key TEXT UNIQUE,
    name TEXT NOT NULL CHECK (length(trim(name)) > 0),
    bio TEXT,
    birth_date TEXT,
    death_date TEXT,
    metadata_synced_at TIMESTAMPTZ,
    works_synced_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE book_authors (
    book_id UUID NOT NULL REFERENCES books (id) ON DELETE CASCADE,
    author_id UUID NOT NULL REFERENCES authors (id) ON DELETE CASCADE,
    position INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
    PRIMARY KEY (book_id, author_id)
);

CREATE INDEX book_authors_author_id_idx ON book_authors (author_id);

CREATE TABLE book_identifiers (
    kind TEXT NOT NULL CHECK (kind IN ('isbn', 'open_library_edition')),
    value TEXT NOT NULL CHECK (length(trim(value)) > 0),
    book_id UUID NOT NULL REFERENCES books (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, value)
);

CREATE INDEX book_identifiers_book_id_idx ON book_identifiers (book_id);

CREATE TABLE library_books (
    library_id UUID NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    book_id UUID NOT NULL REFERENCES books (id) ON DELETE CASCADE,
    added_by_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    title_override TEXT CHECK (length(trim(title_override)) > 0),
    author_override TEXT CHECK (length(trim(author_override)) > 0),
    added_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (library_id, book_id)
);

CREATE INDEX library_books_book_id_idx ON library_books (book_id);

INSERT INTO authors (name)
SELECT DISTINCT author
FROM books;

INSERT INTO book_authors (book_id, author_id)
SELECT books.id, authors.id
FROM books
JOIN authors ON authors.name = books.author;

INSERT INTO library_books (library_id, book_id, added_at)
SELECT library_id, id, created_at
FROM books;

ALTER TABLE books
    DROP COLUMN author,
    DROP COLUMN library_id;

-- +goose Down
CREATE TABLE old_books (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL CHECK (length(trim(title)) > 0),
    author TEXT NOT NULL CHECK (length(trim(author)) > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    library_id UUID NOT NULL REFERENCES libraries (id) ON DELETE CASCADE
);

INSERT INTO old_books (id, title, author, created_at, library_id)
SELECT
    CASE
        WHEN row_number() OVER (PARTITION BY books.id ORDER BY library_books.library_id) = 1 THEN books.id
        ELSE gen_random_uuid()
    END,
    COALESCE(library_books.title_override, books.title),
    COALESCE(library_books.author_override, string_agg(authors.name, ', ' ORDER BY book_authors.position), 'Unknown'),
    library_books.added_at,
    library_books.library_id
FROM library_books
JOIN books ON books.id = library_books.book_id
LEFT JOIN book_authors ON book_authors.book_id = books.id
LEFT JOIN authors ON authors.id = book_authors.author_id
GROUP BY books.id, books.title, library_books.title_override, library_books.author_override, library_books.added_at, library_books.library_id;

DROP TABLE library_books;
DROP TABLE book_identifiers;
DROP TABLE book_authors;
DROP TABLE authors;
DROP TABLE books;
ALTER TABLE old_books RENAME TO books;
CREATE INDEX books_library_id_idx ON books (library_id);
