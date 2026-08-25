{include table_head.tpl}

<div class="facts">
	<dl class="detail">
		<div><dt>Rows</dt><dd>{$rows}</dd></div>
		<div><dt>Columns</dt><dd>{$columns|count}</dd></div>
		<div><dt>Row identity</dt><dd>
			{if $identity.kind == "key"}primary key
			{elseif $identity.kind == "rowid"}sqlite rowid
			{else}none &mdash; rows cannot be edited or deleted{/if}
		</dd></div>
	</dl>
</div>

<h2>Columns</h2>

<div class="scroll">
<table>
	<thead>
		<tr>
			<th class="name flex">Column</th>
			<th>Type</th>
			<th>Null</th>
			<th class="c-medium">Default</th>
			<th>Key</th>
		</tr>
	</thead>
	<tbody>
		{foreach $columns as $column}
		<tr>
			<td class="name flex">{$column.name}</td>
			<td class="dim">{$column.type}</td>
			<td class="dim">{if $column.nullable}yes{else}no{/if}</td>
			<td class="dim c-medium">{if $column.default === null}<span class="null">NULL</span>{else}{$column.default}{/if}</td>
			<td>{if $column.is_key}<span class="status">PK</span>{/if}</td>
		</tr>
		{/foreach}
	</tbody>
</table>
</div>

<h2>Indexes</h2>

{if $indexes}
<div class="scroll">
<table>
	<thead><tr><th class="name flex">Index</th><th>Unique</th><th class="c-medium">Columns</th></tr></thead>
	<tbody>
		{foreach $indexes as $index}
		<tr>
			<td class="name flex">{$index.name}</td>
			<td class="dim">{if $index.is_unique}yes{else}no{/if}</td>
			<td class="dim c-medium"><code class="truncate">{$index.columns}</code></td>
		</tr>
		{/foreach}
	</tbody>
</table>
</div>
{else}
<p class="note">No indexes on this table.</p>
{/if}

{if $definition}
<h2>Definition</h2>
<pre><code>{$definition}</code></pre>
{/if}

{if $is_readonly == false}
<h2>Danger zone</h2>

<div class="danger-zone">
	<form method="post" action="/t/{$table|urlencode}/empty" id="empty" class="danger-form">
		<input type="hidden" name="csrf_token" value="{$ctx.csrf_token}">
		<div>
			<b>Empty this table</b>
			<p>Deletes every row and keeps the table. {$rows} row(s) would go.</p>
		</div>
		<div class="danger-form__confirm">
			<input name="confirmation" placeholder="Type {$table} to confirm" autocomplete="off" data-confirm-name="{$table}">
			<button type="submit" class="danger"{if $ctx.can_destroy == false} disabled{/if}>Empty</button>
		</div>
	</form>

	<form method="post" action="/t/{$table|urlencode}/drop" id="drop" class="danger-form">
		<input type="hidden" name="csrf_token" value="{$ctx.csrf_token}">
		<div>
			<b>Drop this table</b>
			<p>Removes the table and everything in it. There is nothing behind this to undo it.</p>
		</div>
		<div class="danger-form__confirm">
			<input name="confirmation" placeholder="Type {$table} to confirm" autocomplete="off" data-confirm-name="{$table}">
			<button type="submit" class="danger"{if $ctx.can_destroy == false} disabled{/if}>Drop</button>
		</div>
	</form>

	{if $ctx.can_destroy == false}
	<p class="note">
		{if $ctx.offers_toggle}Turn on destructive mode in the sidebar to use these.
		{else}Destructive actions are blocked for your account by policy.{/if}
	</p>
	{/if}
</div>
{/if}
