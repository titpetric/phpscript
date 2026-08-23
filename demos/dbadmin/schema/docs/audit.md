# Audit

Audit.

| Name          | Type     | Key | Comment       |
|---------------|----------|-----|---------------|
| id            | bigint   | PRI | ID            |
| user_id       | bigint   | MUL | User ID       |
| connection_id | bigint   | MUL | Connection ID |
| rel_table     | varchar  | MUL | Rel Table     |
| rel_id        | varchar  | MUL | Rel ID        |
| action        | varchar  |     | Action        |
| message       | varchar  |     | Message       |
| payload       | varchar  |     | Payload       |
| created_at    | datetime | MUL | Created At    |
