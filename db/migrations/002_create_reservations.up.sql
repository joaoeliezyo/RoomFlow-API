BEGIN;

CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE reservations (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    room_id BIGINT NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    cancelled_at TIMESTAMPTZ NULL,
    CONSTRAINT reservations_room_id_fkey FOREIGN KEY (room_id)
        REFERENCES rooms(id) ON DELETE RESTRICT,
    CONSTRAINT reservations_valid_period CHECK (ends_at > starts_at),
    CONSTRAINT reservations_no_overlap EXCLUDE USING gist (
        room_id WITH =,
        tstzrange(starts_at, ends_at, '[)') WITH &&
    ) WHERE (cancelled_at IS NULL)
);

INSERT INTO schema_migrations (version) VALUES (2);

COMMIT;
