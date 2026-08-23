# User

User.

| Name               | Type     | Key | Comment            |
|--------------------|----------|-----|--------------------|
| id                 | bigint   | PRI | ID                 |
| username           | varchar  | MUL | Username           |
| password_hash      | varchar  |     | Password Hash      |
| is_admin           | boolean  | MUL | Is Admin           |
| is_enabled         | boolean  |     | Is Enabled         |
| destructive_policy | varchar  |     | Destructive Policy |
| last_login_at      | datetime |     | Last Login At      |
| created_at         | datetime |     | Created At         |
| updated_at         | datetime |     | Updated At         |
