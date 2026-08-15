CREATE TABLE migration_users (
    id integer primary key,
    name text not null
);

INSERT INTO migration_users (id, name) VALUES (1, 'Ada');
