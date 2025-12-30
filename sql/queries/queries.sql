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

-- name: CreateTag :one
INSERT INTO tags (name)
VALUES (?)
ON CONFLICT(name) DO UPDATE SET name=name
RETURNING *;

-- name: GetTagByName :one
SELECT * FROM tags
WHERE name = ?;

-- name: CreateTimeEntryTag :exec
INSERT INTO time_entry_tags (time_entry_id, tag_id)
VALUES (?, ?);

-- name: ListTagsForTimeEntry :many
SELECT t.* FROM tags t
JOIN time_entry_tags tet ON t.id = tet.tag_id
WHERE tet.time_entry_id = ?;

-- name: DeleteTimeEntryTags :exec
DELETE FROM time_entry_tags
WHERE time_entry_id = ?;


-- name: DeleteTimeEntry :exec
DELETE FROM time_entries
WHERE id = ?;

-- name: DeleteOrphanedTags :exec
DELETE FROM tags
WHERE NOT EXISTS (
    SELECT 1 FROM time_entry_tags WHERE tag_id = tags.id
);
