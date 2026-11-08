-- catalogue holds the demo records the admin console starts with.
--
-- This file is append only. The migrations table records every applied
-- statement by index. Add a new statement at the end and never edit or remove
-- the ones above it.

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
