-- catalogue holds the demo records the admin console starts with.
--
-- This file is append only. Statements already applied are recorded by
-- index in the migrations table, so add new statements at the end and
-- never edit or remove the ones above them.

CREATE TABLE catalogue (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	category TEXT NOT NULL DEFAULT 'General',
	notes TEXT,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO catalogue (name, category, notes) VALUES ('SQLite Handbook', 'Books', 'A sample record you can edit or export.');

INSERT INTO catalogue (name, category, notes) VALUES ('Desk Lamp', 'Equipment', 'Created by the startup migration.');

INSERT INTO catalogue (name, category, notes) VALUES ('Local database', 'Projects', 'Explore this demo using the navigation above.');
