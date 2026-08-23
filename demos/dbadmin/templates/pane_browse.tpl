{include table_head.tpl}

<form class="filter-bar" method="get" action="/t/{$table|urlencode}">
	<input type="search" name="q" value="{$result.search|h}" placeholder="Search every column" id="dbadmin-filter">
	<button type="submit">Search</button>
	{if $result.search}<a class="link" href="/t/{$table|urlencode}">Clear</a>{/if}
	<span class="hint">{$result.total} row(s)</span>
</form>

{if $result.rows}
<div class="scroll">
<table>
	<thead>
		<tr>
			{if $result.identity.kind != "none"}<th class="dim">Row</th>{/if}
			{foreach $result.columns as $column}
			<th class="name">{$column.name|h}<small>{$column.type|h}</small></th>
			{/foreach}
		</tr>
	</thead>
	<tbody>
		{foreach $result.rows as $row}
		{eval $key = browse_dao::key_of($result["identity"], $row)}
		<tr>
			{if $result.identity.kind != "none"}
			<td class="actions dim">
				<a href="/t/{$table|urlencode}/row/{$key|urlencode}/edit">Edit</a>
				{if $ctx.can_destroy}
				<form method="post" action="/t/{$table|urlencode}/row/{$key|urlencode}/delete" data-confirm="Delete this row?">
					<input type="hidden" name="csrf_token" value="{$ctx.csrf_token|h}">
					<button type="submit" class="link danger">Delete</button>
				</form>
				{/if}
			</td>
			{/if}
			{foreach $result.columns as $column}
			<td class="cell">{$row[$column.name]|h}</td>
			{/foreach}
		</tr>
		{/foreach}
	</tbody>
</table>
</div>

{if $result.pages > 1}
<nav class="pager">
	{if $result.page > 1}<a href="/t/{$table|urlencode}?page={$result.page - 1}&amp;q={$result.search|urlencode}">&larr; Previous</a>{/if}
	<span>Page {$result.page} of {$result.pages}</span>
	{if $result.page < $result.pages}<a href="/t/{$table|urlencode}?page={$result.page + 1}&amp;q={$result.search|urlencode}">Next &rarr;</a>{/if}
</nav>
{/if}
{else}
<p class="empty-state">
	<b>No rows</b>
	{if $result.search}Nothing in this table matches that search.{else}This table is empty.{/if}
</p>
{/if}

{if $result.identity.kind == "none"}
<p class="note">This table has no primary key, so a single row cannot be addressed: editing and deleting are off, and
browsing and inserting still work.</p>
{/if}
