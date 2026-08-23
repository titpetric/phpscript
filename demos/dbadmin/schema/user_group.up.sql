-- user_group: A named set of users sharing one grant of connections.
--
-- destructive_policy is combined with user.destructive_policy by acl_dao and
-- the stricter of the two wins, ranked denied < toggle < allowed. A group is
-- therefore a way to tighten an account, never to loosen it.
--
-- Deleting a group deletes its user_group_member and user_group_connection
-- rows.

CREATE TABLE user_group (
	id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
	name TEXT NOT NULL DEFAULT '',
	description TEXT NOT NULL DEFAULT '',
	destructive_policy TEXT NOT NULL DEFAULT 'allowed' CHECK (destructive_policy IN ('denied','toggle','allowed')),
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS uidx_user_group_name ON user_group(name);
