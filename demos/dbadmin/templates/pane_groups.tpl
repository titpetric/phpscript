<div class="pane-head"><div><h1>Groups</h1></div></div>

{include errors.tpl}

<p class="lede">A group is how a non-administrator is granted a connection. It can also tighten what its members may do:
where a group and an account disagree about destructive actions, the stricter of the two wins.</p>

{if $groups}
<div class="scroll">
<table>
	<thead>
		<tr>
			<th class="name">Group</th>
			<th class="flex c-medium">Description</th>
			<th>Destructive</th>
			<th class="num">Members</th>
			<th class="num">Connections</th>
			<th></th>
		</tr>
	</thead>
	<tbody>
		{foreach $groups as $entry}
		<tr>
			<td class="name"><a href="/admin/group/{$entry.id}">{$entry.name|h}</a></td>
			<td class="dim flex c-medium">{$entry.description|hv}</td>
			<td class="dim">{$entry.destructive_policy|h}</td>
			<td class="num">{$entry.members}</td>
			<td class="num">{$entry.connections}</td>
			<td class="actions"><a href="/admin/group/{$entry.id}">Edit</a></td>
		</tr>
		{/foreach}
	</tbody>
</table>
</div>
{else}
<p class="empty-state"><b>No groups</b>Create one below, then grant it a connection and put an account in it.</p>
{/if}

<h2>Add a group</h2>

<form method="post" action="/admin/group" class="stack">
	<input type="hidden" name="csrf_token" value="{$ctx.csrf_token|h}">

	<div class="field">
		<label for="g-name">Name</label>
		<input id="g-name" name="name" value="{$name|h}" autocomplete="off" required>
	</div>
	<div class="field grow">
		<label for="g-desc">Description</label>
		<input id="g-desc" name="description" autocomplete="off">
	</div>
	<div class="field">
		<label for="g-policy">Destructive actions</label>
		<select id="g-policy" name="policy">
			{foreach $policies as $policy}
			<option value="{$policy|h}"{if $policy == "allowed"} selected{/if}>{$policy|h}</option>
			{/foreach}
		</select>
		<small>Leave this on <b>allowed</b> unless the group should be stricter than its members' own settings.</small>
	</div>

	<button type="submit">Add group</button>
</form>
