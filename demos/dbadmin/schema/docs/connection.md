# Connection

Connection.

| Name           | Type     | Key | Comment        |
|----------------|----------|-----|----------------|
| id             | bigint   | PRI | ID             |
| name           | varchar  | MUL | Name           |
| driver         | varchar  |     | Driver         |
| dsn            | varchar  |     | Dsn            |
| default_schema | varchar  |     | Default Schema |
| is_enabled     | boolean  | MUL | Is Enabled     |
| is_readonly    | boolean  |     | Is Readonly    |
| status         | varchar  |     | Status         |
| status_message | varchar  |     | Status Message |
| table_count    | bigint   |     | Table Count    |
| column_count   | bigint   |     | Column Count   |
| schema_count   | bigint   |     | Schema Count   |
| checked_at     | datetime |     | Checked At     |
| created_at     | datetime |     | Created At     |
| updated_at     | datetime |     | Updated At     |
