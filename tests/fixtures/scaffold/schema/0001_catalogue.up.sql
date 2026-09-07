-- catalogue is the table the scaffold fixtures read.
--
-- This file is append only. Statements already applied are recorded by index in
-- the migrations table, so add new statements at the end and never edit or
-- remove the ones above them.
--
-- It creates and does not populate. Rows are the setup hook's, because a row
-- inserted here is applied once and never again, and a fixture that inserts
-- would leave the next run asserting against a table it did not start with.

CREATE TABLE catalogue (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	category TEXT NOT NULL DEFAULT 'General'
);
