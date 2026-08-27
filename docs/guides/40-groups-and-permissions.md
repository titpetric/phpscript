# Groups and permissions

This chapter covers how a grant is stored, how a check resolves, how a module declares the
permissions it manages, and how groups connect an account to a grant. At the end of it you
can add a permission to a module, grant it to a group at the right scope, and gate a route
on it.

## The model

A user reaches a grant only through group membership. A grant is one row in the `rule`
table saying that members of one group have `yes` or `no` for one `role.permission` inside
one module, scoped to one section. Sections are the scoping dimension: a section id is
whatever the module says it is, and in the user panel it is a group id, so the question a
grant answers is "may members of group A edit users in group B". The section `"0"` is the
module-wide default, it sorts below every real section, and it answers only when nothing
more specific has a row. There are three states and two values: `yes` allows, `no` denies,
and the absence of a row inherits from the next lower section, ultimately from `"0"`, and a
check with no row anywhere is denied.

A row in `schema/rule.up.sql` is `(module_id, user_group_id, role, permission, section)`
mapped to `value`, and that tuple is the primary key, so a group cannot hold two values for
one permission at one section. An explicit `'inherit'` value would need a `WHERE` clause in
the evaluation query, and adding a predicate changes which row the ordering lands on, so
inherit stays as the absence of a row.

## How to ask the question

A check is one call, as `routes/admin-users-edit.php` makes it:

```php
$member_of = $groups->group_ids_of($_PATH["id"]);
require_can($html, $rules["user"], "user.edit", $member_of);
```

It has three parts. The first is which `Common\Rules` to ask. There is one per module, built
in the [composition root](25-structuring-an-application.md) as
`$rules[$panel->name()] = new Common\Rules($rule_store, $identity, $panel);` and keyed by
module name. `$rules` is not a container: a route file indexes it with the literal name of
its own panel. The module is a constructor argument rather than a per-call one because
`sections_for()` needs the declaration, not just the name.

The second part is the key, `role.permission`. A key with no dot means the role `default`,
which is the module-wide role.

The third part is the section list, and `Common\Rules` refuses to default it:

```php
$rules->can("blog.edit");
// Exception: Rules: blog.edit is scoped to a section and none was given
```

A caller that forgot the section is asking a different question from the one it thinks it is
asking. The refusal is the first branch in `can_for()`, ahead of the administrator check, so
the mistake is found whoever is signed in. A module-wide question passes the module-wide
section explicitly:

```php
require_can($html, $rules["user"], "user.list", array("0"));
```

An empty list is accepted too, because it is an array. `routes/admin-users-create.php` uses
`array()` for `user.create`: creating a user is not about any existing group, and `"0"` is
appended to the empty list anyway. The class throws `\Exception` with a leading backslash,
because inside `namespace Common;` the unqualified name resolves to `Common\Exception`,
which does not exist, and the resulting error is itself catchable as `Exception`.

## How a check resolves

`Common\Store\SqlRuleStore::value()` assembles one statement across four tables:

```sql
SELECT a.value
  FROM rule a
 INNER JOIN module b ON b.id = a.module_id AND b.name = ?
 INNER JOIN user_group c ON c.id = a.user_group_id
 INNER JOIN user_group_member d ON d.user_group_id = a.user_group_id AND d.user_id = ?
 WHERE a.role = ? AND a.permission = ?
   AND a.section IN (?, ?)
 ORDER BY a.section DESC
 LIMIT 1
```

The `IN` list is built from generated placeholders, one per candidate section, and the call
goes through `call_user_func_array`, because argument unpacking at a call site is a parse
error in this runtime. The rules, in the order they apply:

1. The key is split first, so a dotted key with no section list raises before anything else
   happens.
2. `Identity::is_admin($user_id)`, the seam onto
   [the signed-in user](35-sessions-and-identity.md), short-circuits to `true` before the
   query runs. That cannot be expressed as a grant row, so it is a branch and not data.
3. The candidate list is the caller's sections cast to strings, with `"0"` appended. The
   caller never adds it.
4. `ORDER BY a.section DESC LIMIT 1` takes the most specific reachable row and discards
   every other. `section` is `VARCHAR(26)`, so candidates rank as strings: a section id is a
   26 character identifier and `"0"` sorts below any of them.
5. `yes` allows, `no` denies, and no row at any candidate returns `""`, which the caller
   reads as a deny.

The join on `user_group` carries no predicate beyond existence, which is what drops a grant
whose group has been deleted and why `user_group` has no `is_active` column. Precedence
follows the section id and not the order the caller passed the ids in, and deny does not
automatically win: where a user reaches two sections that both hold a row for one key, the
higher sorting id answers. Group ids are ULIDs, which sort in creation order, so that is the
group created later.

## How to declare what a module manages

A module states which keys exist and what they can be scoped to, through the
`Common\ModuleInfo` methods `roles()` and `sections_by_role($role)`. `Common\UsersAdmin`
declares seven permissions under one role:

```php
public function roles()
{
	return array(
		"user" => array(
			"list" => "List users",
			"read" => "View a user",
			"create" => "Create users",
			"edit" => "Edit users",
			"delete" => "Delete users",
			"password" => "Set a password",
			"group" => "Change group membership",
		),
	);
}
```

The outer key is the role and the inner map is permission name to title, one key per action
in the route table, so a route file's `require_can()` always names a key the permission
screen can grant. `sections_by_role("user")` returns one
`array("id" => $group["id"], "title" => $group["name"])` per group, which is why the section
list a route passes is `group_ids_of()`. Any other role gets an empty list, and an empty list
means the role is evaluated against the module-wide default alone.

Adding a permission is an edit to a module and not to the permission system. `Common\Rules`
never enumerates keys, and `Common\Modules::screen()` builds the administration grid from
these declarations, so a new entry in `roles()` appears as a new row on the permission
screen with no other change. `Common\Modules::sync()` writes the `module` row the query
joins against.

## How to place a user in a group

`Common\Groups` owns group records and membership over two tables. `user_group` holds the
group, with a unique index on the name. `user_group_member` is a join table whose primary
key is `(user_id, user_group_id)`, so calling `add_member()` twice is not an error, and
`set_groups($user_id, $ids)` replaces a user's whole membership set, which is what a user
edit form submits.

`group_ids_of($user_id)` is the hot path. It is the section list a route passes to `can()`,
so it runs on every scoped check, and it reads ids out of the membership table in one
statement rather than taking the keys off `groups_of()`, which would transport whole group
rows to throw them away. `routes/admin-users-edit.php` opens with the two lines at the top of
this chapter, after reading the target row, because a 404 for a user that does not exist is
the more useful answer than a 403.

The sections are the target user's groups, so the grant answers "may this administrator edit
users in group X", and a target in no group is evaluated against `"0"` alone.
`Common\Groups` itself evaluates nothing, reads no user row and writes no `rule` row: a group
is the join between an account and a grant, and keeping both ends out of the class is what
stops the permission schema from sitting behind the group component.

## How to enumerate what a user may reach

`sections($key)` and `sections_for($user_id, $key)` answer "which sections may this user do
this in" without asking once per section. Use them to build a list of what a user may reach,
and `can()` to gate one action.

```php
var_dump($rules->sections("blog.edit"));
// array(2) { [0]=> string(5) "sport" [1]=> string(7) "opinion" }
```

The read pulls every grant the user can reach for the key. If section `"0"` has a value and
the role is not `default`, that value is expanded across every section the module declares;
explicit section rows are overlaid on top; `"0"` is dropped, because it is not a section a
caller can act in; and the sections left holding `yes` are returned. An administrator gets
every declared section and no query runs.

## How to delete a group or a user

`schema/` declares no foreign keys, so nothing cascades. Each delete is an application step,
and the component that owns each table issues its own part inside one transaction.

A group delete removes the group's grants, then its membership rows, then the group.
`routes/admin-groups-delete.php` issues the grants part from the route, because the `rule`
table belongs to the permissions component and a `Common\Groups` holding a second store would
put that schema behind this one:

```php
$db->begin();
try {
	$rule_store->revoke_group($group["id"]);
	$groups->remove($group["id"]);
	$db->commit();
} catch (Exception $failed) {
	$db->rollback();

	throw $failed;
}
```

`Groups::remove()` does the other two, clearing `user_group_member` before deleting the
`user_group` row. Skipping `revoke_group()` leaves grants in the table forever, and a
membership that outlives its group would be inherited by a later group created with the same
id.

A user delete removes the memberships, the sessions, then the user.
`routes/admin-users-delete.php` issues these three inside the same begin, commit and inner
catch:

```php
$groups->remove_user($user["id"]);
$session->revoke_all($user["id"]);
$users->remove($user["id"]);
```

No rule rows are removed with a user, because grants attach to groups and never to users, so
a deleted user's grants stop applying when the membership row goes. The inner catch is what
releases the connection the transaction holds; nothing rolls one back on its own here.

## Two worked scenarios

Both use the real `Common\UsersAdmin` over the memory stores, so the sections are group ids
the group component handed out. `Common\Mock\MemoryRuleStore` is the `rule` table as a list
of arrays with the group join already resolved, so a seeded row names the user directly.
Save this inside `src/`, where the includes resolve, and run it with `phpscript run`:

```php
<?php

include "ModuleInfo.php";
include "AdminPanel.php";
include "Identity.php";
include "RuleStore.php";
include "GroupStore.php";
include "UserStore.php";
include "UserDecorator.php";
include "PropertiesDecorator.php";
include "Groups.php";
include "Users.php";
include "UsersAdmin.php";
include "Mock/MemoryGroupStore.php";
include "Mock/MemoryUserStore.php";
include "Mock/MemoryIdentity.php";
include "Mock/MemoryRuleStore.php";
include "Rules.php";

$groups = new Common\Groups(new Common\Mock\MemoryGroupStore());
$users = new Common\Users(new Common\Mock\MemoryUserStore(), new Common\PropertiesDecorator());

$editors = $groups->create("editors");  // "mem1"
$staff = $groups->create("staff");      // "mem2"

$module = new Common\UsersAdmin($users, $groups);
```

The first scenario is an administrator, user 9, who may edit users in one group and not in
the other. One grant row says so:

```php
$store = new Common\Mock\MemoryRuleStore(array(
	array("module" => "user", "user_id" => "9", "role" => "user", "permission" => "edit", "section" => $editors, "value" => "yes"),
));
$rules = new Common\Rules($store, new Common\Mock\MemoryIdentity("9", false), $module);

var_dump($rules->can("user.edit", array($editors)));  // bool(true)
var_dump($rules->can("user.edit", array($staff)));    // bool(false)
var_dump($rules->can("user.edit", array()));          // bool(false)
```

A target in `editors` is reached by the row. A target in `staff` has no row at `staff` and
none at `"0"`, so the query returns nothing and the check denies. A target in no group is the
same question as the empty list: `"0"` alone, and no row there.

The second scenario allows the key module wide and denies it at one section, which is the
direction the override runs:

```php
$store = new Common\Mock\MemoryRuleStore(array(
	array("module" => "user", "user_id" => "9", "role" => "user", "permission" => "edit", "section" => "0", "value" => "yes"),
	array("module" => "user", "user_id" => "9", "role" => "user", "permission" => "edit", "section" => $staff, "value" => "no"),
));
$rules = new Common\Rules($store, new Common\Mock\MemoryIdentity("9", false), $module);

var_dump($rules->can("user.edit", array($editors)));          // bool(true)
var_dump($rules->can("user.edit", array($staff)));            // bool(false)
var_dump($rules->can("user.edit", array($editors, $staff)));  // bool(false)
var_dump($rules->sections("user.edit"));                      // array(1) { [0]=> string(4) "mem1" }
```

`editors` has no row of its own and falls through to the `yes` at `"0"`. `staff` has one, and
a row at a real section outranks the module-wide default. The third call is a target who
belongs to both groups: `"mem2"` sorts above `"mem1"`, so the `staff` row is the one `LIMIT 1`
keeps and the answer is a deny. `sections()` expands the `"0"` value across both declared
sections, overlays the explicit `no`, drops `"0"`, and returns what is left.

`src/Rules.phpt` pins the same rules against a third module, including the administrator
short-circuit and the refusal of a scoped key with no section. Run it with
`phpscript test src/Rules.phpt`.

## What the design kept

The evaluation is unchanged from the code this package replaces: the join structure, the
section ordering, the branch order in `can_for()` and the deny-by-default all behave as they
did. Only names and portability changed, the store, identity and module arriving through the
constructor rather than a static container, bound placeholders instead of values interpolated
into the SQL text, and `LIMIT 1` instead of the MySQL-only `LIMIT 0,1`. The line by line
analysis against the original is in
[../../demos/common-report/12-permissions.md](../../demos/common-report/12-permissions.md).

Next: [An administration panel](45-an-administration-panel.md).
