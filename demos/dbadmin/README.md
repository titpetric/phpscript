# dbadmin

A database administration front end: accounts, named connections to sqlite,
mysql and postgres, group-based access control, an audit trail, and a two-panel
browser whose destructive actions are off until the session asks for them.

It is the largest application in this repository, and it is here to be one:
everything it does is written in the PHP subset phpscript implements, against
the bindings the standard runtime ships.

```bash
phpscript server ./demos/dbadmin
```

The first person to reach it creates the administrator. There is no seeded
account and no default password.

## What it does

- **Accounts.** The first registration becomes the administrator; after that,
  creating an account is an administrative act. Passwords are bcrypt, through
  the host `password_hash()` binding.
- **Connections.** An administrator adds a named connection with a DSN. The name
  is registered with the runtime when the connection is opened, so a connection
  added through the UI works on the next request rather than the next restart.
- **Groups.** A non-administrator reaches a connection by being in a group that
  was granted it. A grant can be read-only, and a group can tighten what its
  members may do but never loosen it.
- **The two panels.** The left rail is the connection in view and its tables;
  the right is the table list with browse, structure, insert, empty and drop,
  or whichever page you asked for.
- **Destructive mode.** Delete, empty and drop are refused unless the session is
  in destructive mode, which is off at every sign-in and expires after fifteen
  minutes. Whether the switch is offered at all is the administrator's setting
  per account.
- **The audit log.** Every change, and every refusal. An attempt that was
  stopped is worth more in the log than a successful drop, because it is the one
  that says somebody tried.
- **The connection test page.** Every connection, its status, and how many
  tables, columns and schemas the login can see. sqlite reports one schema by
  definition.

## Layout

```text
dbadmin/
├── migrate.php              @startup: applies the schema
├── harden.php               @startup: chmods the metadata database to 0600
├── prune.php                @schedule hourly: sweeps expired sessions
├── bootstrap.php            the composition root, included by every route file
├── lib/
│   ├── shim.php             standard library functions the runtime does not have
│   └── dao.php              the require_once manifest, leaves first
├── modules/
│   ├── <name>.php           a routed controller
│   └── <name>_dao.php       its storage, as class <name>_dao
├── templates/               layout.tpl plus one pane per page
├── schema/*.up.sql          one file per table, append only
└── public/assets/
    ├── css/                 dbadmin.css imports settings.css and components/
    └── js/dbadmin.js        progressive enhancement, no dependencies
```

Each module is a controller and a sidecar that holds its storage. A DAO opens
its own `new Database("dbadmin")` — the provider hands them all the same pool —
and composes the DAOs it needs, so `$this->audit->log(...)` is how a change is
recorded. `audit_dao` and `driver_dao` are the leaves and compose nothing.

`phpscript list ./demos/dbadmin/...` prints the routing table.

## Configuration

One connection, for dbadmin's own storage:

```yaml
env:
  - "PLATFORM_DB_DBADMIN=sqlite://dbadmin.db"
```

Everything else is a row in the `connection` table. The DSNs there are stored in
cleartext, because they have to be replayed to open the connection: `harden.php`
restricts the file to its owner at startup, the DSN is redacted everywhere it is
rendered, and `dbadmin` is a reserved connection name so the tool cannot be
pointed at its own credentials. A database you would not put behind that is a
database this should not hold the password for.

## The parts worth reading

`modules/driver_dao.php` is the smallest file and the most load-bearing. It
holds every difference between the three drivers: identifier quoting, the
placeholder style, how a BLOB is rendered as text and how a timestamp is
formatted. Both of the last two happen in SQL because this runtime has neither
`base64_encode()` nor `date()`.

`modules/session_dao.php` explains why the session is a token and not a user id.
`Session\Manager` stores one opaque string and its only writer mints a new
cookie, so the string it stores has to be immutable and everything that changes
during a session is a column on the row it points at.

`lib/shim.php` is the list of standard library functions this runtime does not
register, with the reason each one is written the way it is. A user-defined
function shadows a builtin of the same name, so they keep their PHP spellings
and stop being the ones that run if the runtime grows them.

## Tests

```bash
atkins compose:up          # from the repository root
cd demos/dbadmin/tests && atkins test
```

The suite runs against the compose service and walks browse, structure, insert,
edit, export and search on **all three drivers**. That is deliberate: a suite
that ran on sqlite alone would pass while every bound query against postgres
failed, because postgres numbers its placeholders and sqlite does not.

`atkins reset` drops the metadata database first, because the suite's first
assertion is that the first registration becomes the administrator, and that is
a question an installation can only answer once.
