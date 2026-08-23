<div class="pane-head">
	<div>
		<div class="eyebrow"><a href="/admin/user">Users</a></div>
		<h1>{$user.username|h}</h1>
	</div>
</div>

{include errors.tpl}

<dl class="detail">
	<div><dt>Role</dt><dd>{if $user.is_admin}administrator{else}user{/if}</dd></div>
	<div><dt>Enabled</dt><dd>{if $user.is_enabled}yes{else}no{/if}</dd></div>
	<div><dt>Destructive</dt><dd>{$user.destructive_policy|h}</dd></div>
	<div><dt>Last sign-in</dt><dd>{$user.last_login_at|hv}</dd></div>
	<div><dt>Created</dt><dd>{$user.created_at|hv}</dd></div>
</dl>

<h2>Profile</h2>

<form method="post" action="/admin/user/{$user.id}" class="stack">
	<input type="hidden" name="csrf_token" value="{$ctx.csrf_token|h}">
	<input type="hidden" name="action" value="profile">

	<div class="field">
		<label for="u-policy">Destructive actions</label>
		<select id="u-policy" name="policy">
			{foreach $policies as $policy}
			<option value="{$policy|h}"{if $policy == $user.destructive_policy} selected{/if}>{$policy|h}</option>
			{/foreach}
		</select>
	</div>
	<label class="check"><input type="checkbox" name="is_admin" value="1"{if $user.is_admin} checked{/if}> Administrator</label>
	<label class="check"><input type="checkbox" name="is_enabled" value="1"{if $user.is_enabled} checked{/if}> Enabled</label>
	<small>Disabling an account signs out every session it has.</small>

	<button type="submit">Save profile</button>
</form>

<h2>Groups</h2>

<form method="post" action="/admin/user/{$user.id}" class="stack">
	<input type="hidden" name="csrf_token" value="{$ctx.csrf_token|h}">
	<input type="hidden" name="action" value="groups">

	{foreach $groups as $group}
	<label class="check"><input type="checkbox" name="group_{$group.id}" value="1"{if in_array($group.id, $member_of)} checked{/if}>
		{$group.name|h} <span class="dim">({$group.connections} connection(s), destructive: {$group.destructive_policy|h})</span></label>
	{else}
	<p class="note">No groups exist yet. <a href="/admin/group">Create one</a> to grant this account a connection.</p>
	{/foreach}

	{if $groups}<button type="submit">Save groups</button>{/if}
</form>

<h2>Password</h2>

<form method="post" action="/admin/user/{$user.id}" class="stack">
	<input type="hidden" name="csrf_token" value="{$ctx.csrf_token|h}">
	<input type="hidden" name="action" value="password">

	<div class="field">
		<label for="u-pass">New password</label>
		<input id="u-pass" name="password" type="password" autocomplete="new-password" required>
		<small>Changing it signs out that account's sessions; a password change that left them running would not have
		changed anything for whoever was using them.</small>
	</div>

	<button type="submit">Change password</button>
</form>

<h2>Sessions</h2>

{if $sessions}
<div class="scroll">
<table>
	<thead><tr><th class="name">From</th><th class="c-medium flex">Agent</th><th>Started</th><th class="c-wide">Expires</th></tr></thead>
	<tbody>
		{foreach $sessions as $entry}
		<tr>
			<td class="name">{$entry.remote_addr|hv}</td>
			<td class="dim c-medium flex"><code class="truncate">{$entry.user_agent|hv}</code></td>
			<td class="dim">{$entry.created_at|hv}</td>
			<td class="dim c-wide">{$entry.expires_at|hv}</td>
		</tr>
		{/foreach}
	</tbody>
</table>
</div>
{else}
<p class="note">No live sessions.</p>
{/if}

<h2>History</h2>

{if $log}
<div class="scroll">
<table>
	<thead><tr><th>When</th><th>Action</th><th class="flex">What</th><th class="c-wide">By</th></tr></thead>
	<tbody>
		{foreach $log as $entry}
		<tr>
			<td class="dim">{$entry.created_at|hv}</td>
			<td><span class="status">{$entry.action|h}</span></td>
			<td class="flex">{$entry.message|h}</td>
			<td class="dim c-wide">{$entry.username|hv}</td>
		</tr>
		{/foreach}
	</tbody>
</table>
</div>
{else}
<p class="note">Nothing recorded for this account yet.</p>
{/if}

{if $is_self == false}
<h2>Danger zone</h2>

<div class="danger-zone">
	<form method="post" action="/admin/user/{$user.id}/delete" class="danger-form" data-confirm="Delete {$user.username|h}?">
		<input type="hidden" name="csrf_token" value="{$ctx.csrf_token|h}">
		<div>
			<b>Delete this account</b>
			<p>Removes the account, its group memberships and its sessions. Its audit rows stay: they are the only
			record that it existed.</p>
		</div>
		<div class="danger-form__confirm"><button type="submit" class="danger">Delete</button></div>
	</form>
</div>
{/if}
