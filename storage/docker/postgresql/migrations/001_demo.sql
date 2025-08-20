-- +goose NO TRANSACTION

-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
-- +goose ENVSUB ON
    RAISE NOTICE 'Migration Up on "Patroni node % Pgpool-II backend id %" at %',
        (SELECT client_hostname FROM pg_stat_activity WHERE pid = pg_backend_pid()),
        '${PGPOOL_BACKEND_NODE_ID?Pgpool-II backend node id env var is missing}',
        NOW();
-- +goose ENVSUB OFF
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
-- +goose ENVSUB ON
    RAISE NOTICE 'Migration Down on "Patroni node % Pgpool-II backend id %" at %',
        (SELECT client_hostname FROM pg_stat_activity WHERE pid = pg_backend_pid()),
        '${PGPOOL_BACKEND_NODE_ID?Pgpool-II backend node id env var is missing}',
        NOW();
-- +goose ENVSUB OFF
END $$;
-- +goose StatementEnd

