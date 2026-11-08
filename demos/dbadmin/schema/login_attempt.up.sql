-- login_attempt holds one row per refused sign-in. login.php reads it to bound
-- how many bcrypt derivations one address or one account can demand. It prunes
-- rows older than its window on every post and deletes an address's and an
-- account's rows when it accepts a credential, which keeps the table at a few
-- seconds of refusals.
--
-- This file is append only. The migrations table records every applied
-- statement by index. Add a new statement at the end and never edit or remove
-- the ones above it.

CREATE TABLE login_attempt (
	address TEXT NOT NULL,
	username TEXT NOT NULL,
	attempted_at INTEGER NOT NULL
);

CREATE INDEX idx_login_attempt_address ON login_attempt (address, attempted_at);

CREATE INDEX idx_login_attempt_username ON login_attempt (username, attempted_at);
