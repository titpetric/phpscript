<div class="pane-head"><div><h1>Users</h1></div></div>

{include errors.tpl}

<div class="scroll">
<table>
	<thead>
		<tr>
			<th class="name">Username</th>
			<th>Role</th>
			<th>Destructive</th>
			<th class="c-medium flex">Groups</th>
			<th class="num c-wide">Sessions</th>
			<th class="c-wide">Last sign-in</th>
			<th></th>
		</tr>
	</thead>
	<tbody>
		{foreach $users as $entry}
		<tr>
			<td class="name"><a href="/admin/user/{$entry.id}">{$entry.username}</a></td>
			<td>{if $entry.is_admin}<span class="status">admin</span>{else}<span class="dim">user</span>{/if}
				{if $entry.is_enabled == 0}<span class="status s4xx">disabled</span>{/if}</td>
			<td class="dim">{$entry.destructive_policy}</td>
			<td class="dim c-medium flex">{$entry.groups}</td>
			<td class="num c-wide">{$entry.sessions}</td>
			<td class="dim c-wide">{$entry.last_login_at}</td>
			<td class="actions"><a href="/admin/user/{$entry.id}">Edit</a></td>
		</tr>
		{/foreach}
	</tbody>
</table>
</div>

<h2>Add a user</h2>

<form method="post" action="/admin/user" class="stack">
	<input type="hidden" name="csrf_token" value="{$ctx.csrf_token}">

	<div class="field">
		<label for="u-name">Username</label>
		<input id="u-name" name="username" value="{$username}" autocomplete="off" required>
	</div>
	<div class="field">
		<label for="u-pass">Password</label>
		<input id="u-pass" name="password" type="password" autocomplete="new-password" required>
	</div>
	<div class="field">
		<label for="u-policy">Destructive actions</label>
		<select id="u-policy" name="policy">
			{foreach $policies as $policy}
			<option value="{$policy}"{if $policy == "toggle"} selected{/if}>{$policy}</option>
			{/foreach}
		</select>
		<small><b>denied</b> hides the switch entirely &middot; <b>toggle</b> offers it, off at every sign-in &middot;
		<b>allowed</b> needs no switch.</small>
	</div>
	<label class="check"><input type="checkbox" name="is_admin" value="1"> Administrator</label>

	<button type="submit">Add user</button>
</form>
