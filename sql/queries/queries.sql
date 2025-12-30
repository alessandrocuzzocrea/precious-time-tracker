-- name: CreateTimeEntry :one
INSERT INTO time_entries (
    description,
    start_time
) VALUES (
    ?, ?
)
RETURNING *;

-- name: UpdateTimeEntry :one
UPDATE time_entries
SET end_time = ?
WHERE id = ?
RETURNING *;

-- name: ListTimeEntries :many
SELECT * FROM time_entries
ORDER BY start_time DESC
LIMIT 50;

-- name: GetActiveTimeEntry :one
SELECT * FROM time_entries
WHERE end_time IS NULL
ORDER BY start_time DESC
LIMIT 1;

-- name: UpdateTimeEntryFull :one
UPDATE time_entries
SET description = ?, start_time = ?, end_time = ?
WHERE id = ?
RETURNING *;

-- name: GetTimeEntry :one
SELECT * FROM time_entries
WHERE id = ?;
