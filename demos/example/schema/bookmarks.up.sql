-- bookmarks holds the links the application starts with.
--
-- This file is append only: applied statements are recorded by index in the
-- migrations table, so a schema change is added at the end of the file.

CREATE TABLE bookmarks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT NOT NULL,
	url TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO bookmarks (title, url) VALUES ('phpscript', 'https://github.com/titpetric/phpscript');
