<div class="pane-head">
	<div>
		<div class="eyebrow">{$ctx.driver}{if $ctx.schema_name} &middot; {$ctx.schema_name}{/if}</div>
		<h1>{$ctx.connection_name}</h1>
	</div>
	<div class="pane-head__actions">
		<a href="/sql">SQL console</a>
		{if $is_readonly == false}<a href="/table/create">+ New table</a>{/if}
	</div>
</div>

{if $error}
<p class="notice notice--bad" role="alert">{$error}</p>
{/if}

{if $is_readonly}
<p class="notice">This connection is read-only for you. Browsing and structure work; writing does not.</p>
{/if}

{if $tables}
<div class="scroll">
<table>
	<thead>
		<tr>
			<th class="name">Table</th>
			<th class="num c-medium">Columns</th>
			<th class="num">Rows</th>
			<th class="c-wide dim">Kind</th>
			<th>Actions</th>
		</tr>
	</thead>
	<tbody>
		{foreach $tables as $entry}
		<tr>
			<td class="name"><a href="/t/{$entry.name|urlencode}">{$entry.name}</a></td>
			<td class="num c-medium">{$entry.columns}</td>
			<td class="num">{if $counted}{$entry.row_count}{else}&mdash;{/if}</td>
			<td class="c-wide dim">{$entry.kind}</td>
			<td class="actions">
				<a href="/t/{$entry.name|urlencode}">Browse</a>
				<a href="/t/{$entry.name|urlencode}/structure">Structure</a>
				{if $is_readonly == false}
				<a href="/t/{$entry.name|urlencode}/insert">Insert</a>
				{/if}
				<a href="/t/{$entry.name|urlencode}/export">Export</a>
				{if $is_readonly == false}
				<a class="danger" href="/t/{$entry.name|urlencode}/structure#empty">Empty</a>
				<a class="danger" href="/t/{$entry.name|urlencode}/structure#drop">Drop</a>
				{/if}
			</td>
		</tr>
		{/foreach}
	</tbody>
</table>
</div>

{if $counted == false}
<p class="note">Row counts are not shown: this schema has more tables than the page will count one at a time.</p>
{/if}
{elseif $error == ""}
<p class="empty-state">
	<b>No tables</b>
	This database has no tables yet. {if $is_readonly == false}<a href="/table/create">Create one</a>.{/if}
</p>
{/if}
