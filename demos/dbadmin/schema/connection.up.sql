-- connection: One named target database an administrator exposes to dbadmin.
--
-- name is what Database::register() registers and new Database() then opens,
-- so it is lowercased and unique. dsn is "<driver>://<dsn>" and holds
-- credentials in cleartext; harden.php chmods this database to 0600, and
-- connection_dao::redact_dsn() strips the userinfo before anything is rendered.
--
-- driver selects the introspection dialect. It is stored rather than parsed
-- out of the dsn on every request, and is validated against the dsn on save.
--
-- default_schema is the schema browsed when a session picks this connection:
-- empty on sqlite, a database name on mysql, a schema name on postgres.
--
-- status, status_message and the three counts are what /admin/connection/test
-- last observed. They are cached so the connection list renders without
-- reaching every remote database on every request; checked_at says how stale
-- that is.
--
-- Deleting a connection deletes its user_group_connection rows and clears the
-- connection_id of any session pointing at it.

CREATE TABLE connection (
	id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
	name TEXT NOT NULL DEFAULT '',
	driver TEXT NOT NULL DEFAULT 'sqlite' CHECK (driver IN ('sqlite','mysql','postgres')),
	dsn TEXT NOT NULL DEFAULT '',
	default_schema TEXT NOT NULL DEFAULT '',
	is_enabled BOOLEAN NOT NULL DEFAULT 1,
	is_readonly BOOLEAN NOT NULL DEFAULT 0,
	status TEXT NOT NULL DEFAULT 'unknown' CHECK (status IN ('unknown','ok','error')),
	status_message TEXT NOT NULL DEFAULT '',
	table_count INTEGER NOT NULL DEFAULT 0,
	column_count INTEGER NOT NULL DEFAULT 0,
	schema_count INTEGER NOT NULL DEFAULT 0,
	checked_at DATETIME,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS uidx_connection_name ON connection(name);
CREATE INDEX IF NOT EXISTS idx_connection_is_enabled_name ON connection(is_enabled, name);
