-- user_group_connection: Grants one user_group access to one connection.
--
-- A join table, so the natural pair is the primary key and there is no
-- surrogate id. is_readonly narrows a grant further than connection.is_readonly
-- does, which is how one group reads production while another writes staging.
--
-- Where a user is in several groups that reach the same connection, the
-- loosest read/write grant wins; destructive_policy on the groups is resolved
-- the other way, strictest first. acl_dao owns both rules.
--
-- Rows are deleted by the application when either side goes away.

CREATE TABLE user_group_connection (
	user_group_id INTEGER NOT NULL DEFAULT 0,
	connection_id INTEGER NOT NULL DEFAULT 0,
	is_readonly BOOLEAN NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (user_group_id, connection_id)
);

CREATE INDEX IF NOT EXISTS idx_user_group_connection_connection_id ON user_group_connection(connection_id);
