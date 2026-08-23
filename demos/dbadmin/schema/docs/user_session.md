# User Session

User Session.

| Name              | Type     | Key | Comment           |
|-------------------|----------|-----|-------------------|
| id                | bigint   | PRI | ID                |
| token             | varchar  | MUL | Token             |
| csrf_token        | varchar  |     | Csrf Token        |
| user_id           | bigint   | MUL | User ID           |
| connection_id     | bigint   | MUL | Connection ID     |
| schema_name       | varchar  |     | Schema Name       |
| is_destructive    | boolean  |     | Is Destructive    |
| destructive_until | datetime |     | Destructive Until |
| is_revoked        | boolean  |     | Is Revoked        |
| remote_addr       | varchar  |     | Remote Addr       |
| user_agent        | varchar  |     | User Agent        |
| flash             | varchar  |     | Flash             |
| expires_at        | datetime | MUL | Expires At        |
| created_at        | datetime | MUL | Created At        |
| updated_at        | datetime |     | Updated At        |
