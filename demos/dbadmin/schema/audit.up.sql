-- audit: One recorded event, either an administrative change or a write to a
-- target database.
--
-- Append only: rows are never updated, so there is no updated_at.
--
-- action and message are separate on purpose. action is the SQL verb, or the
-- kind of event where there is no statement, and is what "show me every delete"
-- filters on. message is the sentence a human reads, such as
-- "created new user: bob". One column holding either would be neither.
--
-- rel_table and rel_id are polymorphic: a dbadmin metadata table and its
-- integer id, or a target-database table and its primary key rendered as text.
-- rel_id is TEXT because a target key is not always a number, and it is not a
-- <table>_id reference.
--
-- payload is json_encode() output. SQLite has no JSON type, and jsonb is a
-- type mig cannot generate a model for, so TEXT is also the portable choice.
--
-- user_id and connection_id are 0 for events with no actor or no target, which
-- is why they are NOT NULL with a default rather than nullable.

CREATE TABLE audit (
	id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
	user_id INTEGER NOT NULL DEFAULT 0,
	connection_id INTEGER NOT NULL DEFAULT 0,
	rel_table TEXT NOT NULL DEFAULT '',
	rel_id TEXT NOT NULL DEFAULT '',
	action TEXT NOT NULL DEFAULT 'admin' CHECK (action IN ('select','insert','update','delete','truncate','drop','create','alter','login','logout','denied','admin')),
	message TEXT NOT NULL DEFAULT '',
	payload TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_user_id_created_at ON audit(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_audit_connection_id_created_at ON audit(connection_id, created_at);
CREATE INDEX IF NOT EXISTS idx_audit_rel_table_rel_id ON audit(rel_table, rel_id);
CREATE INDEX IF NOT EXISTS idx_audit_created_at ON audit(created_at);
