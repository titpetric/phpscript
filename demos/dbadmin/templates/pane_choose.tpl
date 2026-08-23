<h1>Connections</h1>

{if $connections}
<p class="lede">Pick a database to work in. Switching connection leaves destructive mode, if you were in it.</p>

<div class="scroll">
<table>
	<thead>
		<tr>
			<th class="name">Connection</th>
			<th>Driver</th>
			<th>Status</th>
			<th class="num c-medium">Tables</th>
			<th class="c-wide">Schema</th>
			<th></th>
		</tr>
	</thead>
	<tbody>
		{foreach $connections as $entry}
		<tr>
			<td class="name">{$entry.name|h}</td>
			<td class="dim">{$entry.driver|h}</td>
			<td>
				{if $entry.status == "ok"}<span class="dot dot--ok"></span>OK
				{elseif $entry.status == "error"}<span class="dot dot--bad"></span>{$entry.status_message|h}
				{else}<span class="dot"></span>Not tested{/if}
			</td>
			<td class="num c-medium">{$entry.table_count}</td>
			<td class="dim c-wide">{$entry.default_schema|h}</td>
			<td>
				<form method="post" action="/session/connection">
					<input type="hidden" name="csrf_token" value="{$ctx.csrf_token|h}">
					<input type="hidden" name="connection_id" value="{$entry.id}">
					<button type="submit">Open</button>
				</form>
			</td>
		</tr>
		{/foreach}
	</tbody>
</table>
</div>
{else}
<p class="empty-state">
	<b>No connections</b>
	{if $ctx.is_admin}
	You have not added a connection yet. <a href="/admin/connection">Add one</a> to start browsing a database.
	{else}
	No connection has been granted to your account. An administrator has to put you in a group that reaches one.
	{/if}
</p>
{/if}
