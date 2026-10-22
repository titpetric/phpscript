-- login_attempt holds one row per refused sign-in, so login.php can bound how
-- many bcrypt derivations one address or one account can demand. login.php
-- prunes rows older than its window on every post and deletes an address's
-- and an account's rows when it accepts a credential, so the table holds at
-- most a few seconds of refusals.
--
-- This file is append only. Statements already applied are recorded by
-- index in the migrations table, so add new statements at the end and
-- never edit or remove the ones above them.

CREATE TABLE login_attempt (
	address TEXT NOT NULL,
	username TEXT NOT NULL,
	attempted_at INTEGER NOT NULL
);

CREATE INDEX idx_login_attempt_address ON login_attempt (address, attempted_at);

CREATE INDEX idx_login_attempt_username ON login_attempt (username, attempted_at);
