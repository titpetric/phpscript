-- user: One dbadmin login. The first row ever inserted becomes the administrator.
--
-- password_hash holds what password_hash() produced: a bcrypt string carrying
-- its own salt and cost, so there is no salt column and no algorithm column.
-- A cost change is picked up by password_needs_rehash() at the next login.
--
-- destructive_policy is the administrator's decision about this account:
--   denied   the destructive-mode toggle is not offered, and a hand-made POST
--            to it is refused
--   toggle   the toggle is offered, and starts off at every login
--   allowed  destructive statements run without a toggle
--
-- The name "user" is reserved on PostgreSQL. This database is SQLite and the
-- word is not reserved in MySQL 8.0, so it stands; the target databases
-- dbadmin browses are a different matter and are always quoted.
--
-- Deleting a user deletes its user_group_member and user_session rows. That is
-- the application's job: there are no foreign keys anywhere in this schema.

CREATE TABLE user (
	id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
	username TEXT NOT NULL DEFAULT '',
	password_hash TEXT NOT NULL DEFAULT '',
	is_admin BOOLEAN NOT NULL DEFAULT 0,
	is_enabled BOOLEAN NOT NULL DEFAULT 1,
	destructive_policy TEXT NOT NULL DEFAULT 'toggle' CHECK (destructive_policy IN ('denied','toggle','allowed')),
	last_login_at DATETIME,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS uidx_user_username ON user(username);
CREATE INDEX IF NOT EXISTS idx_user_is_admin_username ON user(is_admin, username);
