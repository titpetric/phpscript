<div class="pane-head">
	<div><h1>Audit log</h1></div>
	<div class="pane-head__actions"><span class="hint">{$total} event(s)</span></div>
</div>

<form class="filter-bar" method="get" action="/admin/audit">
	<div class="field">
		<label for="a-action">Action</label>
		<select id="a-action" name="action">
			<option value="">any</option>
			{foreach $actions as $action}
			<option value="{$action|h}"{if $action == $filter.action} selected{/if}>{$action|h}</option>
			{/foreach}
		</select>
	</div>
	<div class="field">
		<label for="a-user">User</label>
		<select id="a-user" name="user">
			<option value="0">anyone</option>
			{foreach $actors.users as $actor}
			<option value="{$actor.id}"{if $actor.id == $filter.user_id} selected{/if}>{$actor.username|hv}</option>
			{/foreach}
		</select>
	</div>
	<div class="field">
		<label for="a-conn">Connection</label>
		<select id="a-conn" name="connection">
			<option value="0">any</option>
			{foreach $actors.connections as $actor}
			<option value="{$actor.id}"{if $actor.id == $filter.connection_id} selected{/if}>{$actor.name|hv}</option>
			{/foreach}
		</select>
	</div>
	<div class="field">
		<label for="a-table">Table</label>
		<input id="a-table" name="table" value="{$filter.rel_table|h}" autocomplete="off">
	</div>
	<button type="submit">Filter</button>
</form>

{if $log}
<div class="scroll">
<table>
	<thead>
		<tr>
			<th>When</th>
			<th>Action</th>
			<th class="name">Who</th>
			<th class="c-medium">Where</th>
			<th class="flex">What</th>
			<th class="c-wide">Detail</th>
		</tr>
	</thead>
	<tbody>
		{foreach $log as $entry}
		<tr>
			<td class="dim">{$entry.created_at|hv}</td>
			<td><span class="status{if $entry.action == "denied"} s4xx{elseif $entry.action == "drop"} s5xx{elseif $entry.action == "truncate"} s5xx{/if}">{$entry.action|h}</span></td>
			<td class="name">{if $entry.username}{$entry.username|h}{elseif $entry.user_id}<span class="dim">#{$entry.user_id}</span>{else}<span class="dim">&mdash;</span>{/if}</td>
			<td class="dim c-medium">{if $entry.connection_name}{$entry.connection_name|h}{/if}{if $entry.rel_table} / {$entry.rel_table|h}{/if}</td>
			<td class="flex">{$entry.message|h}</td>
			<td class="dim c-wide"><code class="truncate">{$entry.payload|hv}</code></td>
		</tr>
		{/foreach}
	</tbody>
</table>
</div>

{if $pages > 1}
<nav class="pager">
	{if $page > 1}<a href="/admin/audit?page={$page - 1}">&larr; Previous</a>{/if}
	<span>Page {$page} of {$pages}</span>
	{if $page < $pages}<a href="/admin/audit?page={$page + 1}">Next &rarr;</a>{/if}
</nav>
{/if}
{else}
<p class="empty-state"><b>Nothing recorded</b>No event matches this filter.</p>
{/if}
