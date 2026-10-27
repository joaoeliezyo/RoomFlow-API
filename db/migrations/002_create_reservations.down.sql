BEGIN;

DROP TABLE reservations;
DELETE FROM schema_migrations WHERE version = 2;

COMMIT;
