-- bookmarks holds the links the application starts with.
--
-- This file is append only. The migrations table records every applied
-- statement by index. A schema change goes at the end of the file.

CREATE TABLE bookmarks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT NOT NULL,
	url TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO bookmarks (title, url) VALUES ('phpscript', 'https://github.com/titpetric/phpscript');
