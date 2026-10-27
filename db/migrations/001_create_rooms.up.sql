BEGIN;

CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY
);

CREATE TABLE rooms (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name TEXT NOT NULL CHECK (btrim(name) <> ''),
    capacity INTEGER NOT NULL CHECK (capacity > 0)
);

INSERT INTO schema_migrations (version) VALUES (1);

COMMIT;
