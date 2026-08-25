<div class="pane-head">
	<div>
		<div class="eyebrow"><a href="/tables">{$ctx.connection_name}</a> &middot; {$ctx.driver}</div>
		<h1>SQL console</h1>
	</div>
</div>

{include errors.tpl}

{if $is_readonly}
<p class="notice">This connection is read-only for you: SELECT, SHOW and DESCRIBE run, and nothing else does.</p>
{/if}

<form method="post" action="/sql" class="stack">
	<input type="hidden" name="csrf_token" value="{$ctx.csrf_token}">
	<textarea name="statement" rows="6" spellcheck="false" autofocus placeholder="SELECT * FROM ...">{$statement}</textarea>
	<div class="form-actions">
		<button type="submit">Run</button>
		{if $kind}<span class="hint">Last statement classified as <b>{$kind}</b></span>{/if}
	</div>
</form>

{if $result.message}
<p class="notice notice--ok">{$result.message}</p>
{/if}

{if $result.rows}
<div class="scroll">
<table>
	<thead>
		<tr>{foreach $result.columns as $column}<th class="name">{$column}</th>{/foreach}</tr>
	</thead>
	<tbody>
		{foreach $result.rows as $row}
		<tr>{foreach $result.columns as $column}<td class="cell">{if $row[$column] === null}<span class="null">NULL</span>{else}{$row[$column]}{/if}</td>{/foreach}</tr>
		{/foreach}
	</tbody>
</table>
</div>
<p class="note">Columns are listed alphabetically. A result row arrives from the driver as a map with no column order,
so the order the statement asked for is not one this page can recover; naming the columns in the SELECT and reading them
by name is what a script does instead.</p>
{/if}
