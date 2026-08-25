<div class="pane-head">
	<div>
		<div class="eyebrow"><a href="/admin/group">Groups</a></div>
		<h1>{$group.name}</h1>
	</div>
</div>

{include errors.tpl}

<h2>Settings</h2>

<form method="post" action="/admin/group/{$group.id}" class="stack">
	<input type="hidden" name="csrf_token" value="{$ctx.csrf_token}">
	<input type="hidden" name="action" value="profile">

	<div class="field">
		<label for="g-name">Name</label>
		<input id="g-name" name="name" value="{$group.name}" autocomplete="off" required>
	</div>
	<div class="field grow">
		<label for="g-desc">Description</label>
		<input id="g-desc" name="description" value="{$group.description}" autocomplete="off">
	</div>
	<div class="field">
		<label for="g-policy">Destructive actions</label>
		<select id="g-policy" name="policy">
			{foreach $policies as $policy}
			<option value="{$policy}"{if $policy == $group.destructive_policy} selected{/if}>{$policy}</option>
			{/foreach}
		</select>
	</div>

	<button type="submit">Save</button>
</form>

<h2>Connections</h2>

<form method="post" action="/admin/group/{$group.id}" class="stack">
	<input type="hidden" name="csrf_token" value="{$ctx.csrf_token}">
	<input type="hidden" name="action" value="grants">

	{if $connections}
	<div class="scroll">
	<table>
		<thead><tr><th class="name">Connection</th><th>Granted</th><th>Read-only</th><th class="dim c-medium flex">Driver</th></tr></thead>
		<tbody>
			{foreach $connections as $entry}
			<tr>
				<td class="name">{$entry.name}</td>
				<td><input type="checkbox" name="grant_{$entry.id}" value="1" aria-label="Granted"{if $granted[$entry.id]} checked{/if}></td>
				<td><input type="checkbox" name="ro_{$entry.id}" value="1" aria-label="Read-only"{if $readonly[$entry.id]} checked{/if}></td>
				<td class="dim c-medium flex">{$entry.driver}</td>
			</tr>
			{/foreach}
		</tbody>
	</table>
	</div>
	<button type="submit">Save connections</button>
	{else}
	<p class="note">No connections exist yet. <a href="/admin/connection">Add one</a> first.</p>
	{/if}
</form>

<h2>Members</h2>

<form method="post" action="/admin/group/{$group.id}" class="stack">
	<input type="hidden" name="csrf_token" value="{$ctx.csrf_token}">
	<input type="hidden" name="action" value="members">

	{foreach $users as $entry}
	<label class="check"><input type="checkbox" name="user_{$entry.id}" value="1"{if in_array($entry.id, $member_ids)} checked{/if}>
		{$entry.username}{if $entry.is_admin} <span class="status">admin</span>{/if}</label>
	{/foreach}

	<button type="submit">Save members</button>
</form>

<h2>Danger zone</h2>

<div class="danger-zone">
	<form method="post" action="/admin/group/{$group.id}/delete" class="danger-form" data-confirm="Delete {$group.name}?">
		<input type="hidden" name="csrf_token" value="{$ctx.csrf_token}">
		<div>
			<b>Delete this group</b>
			<p>Removes the group, its memberships and its grants. Members lose whatever access this group gave them.</p>
		</div>
		<div class="danger-form__confirm"><button type="submit" class="danger">Delete</button></div>
	</form>
</div>
