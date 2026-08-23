-- user_group_member: Places one user in one user_group.
--
-- A join table, so the natural pair is the primary key and there is no
-- surrogate id. The pair also gives user_id the leading position in an index,
-- which is the hot path: every request resolves the logged-in user's groups.
-- idx_user_group_member_user_group_id answers the other direction, for the
-- group edit page and for the delete cascade.
--
-- Rows are deleted by the application when either side goes away.

CREATE TABLE user_group_member (
	user_id INTEGER NOT NULL DEFAULT 0,
	user_group_id INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (user_id, user_group_id)
);

CREATE INDEX IF NOT EXISTS idx_user_group_member_user_group_id ON user_group_member(user_group_id);
