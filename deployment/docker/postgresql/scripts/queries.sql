-- queries.sql

-- --
-- User Management
-- --

-- name: CreateUser :one
INSERT INTO "user" (
    email,
    password_hash,
    password_algo,
    name
) VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM "user" WHERE email = $1;

-- name: GetAllUsers :many
SELECT * FROM "user";

-- name: DeleteUserByEmail :exec
DELETE FROM "user" WHERE email = $1;