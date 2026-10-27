BEGIN;

DROP TABLE rooms;
DELETE FROM schema_migrations WHERE version = 1;
DROP TABLE schema_migrations;

COMMIT;
