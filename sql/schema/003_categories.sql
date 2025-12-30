-- +goose Up
CREATE TABLE categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    color TEXT NOT NULL DEFAULT '#cccccc',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE time_entries ADD COLUMN category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL;

CREATE INDEX idx_time_entries_category_id ON time_entries(category_id);

-- +goose Down
DROP INDEX idx_time_entries_category_id;
-- SQLite doesn't support easy DROP COLUMN, so we'll just leave it if it's down,
-- but we should ideally recreate the table if we really wanted to.
-- For simple migrations, we often just leave it or use a tool.
-- However, for the sake of completeness in a migration script:
-- (Actually, ALTER TABLE DROP COLUMN is supported in SQLite 3.35.0+)
ALTER TABLE time_entries DROP COLUMN category_id;
DROP TABLE categories;
