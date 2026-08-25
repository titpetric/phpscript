<div class="pane-head">
	<div><h1>Connections</h1></div>
	<div class="pane-head__actions"><a href="/admin/connection/test">Test all</a></div>
</div>

{include errors.tpl}

{if $connections}
<div class="scroll">
<table>
	<thead>
		<tr>
			<th class="name">Name</th>
			<th>Driver</th>
			<th>Status</th>
			<th class="num c-medium">Tables</th>
			<th class="num c-wide">Grants</th>
			<th class="flex c-wide">DSN</th>
			<th></th>
		</tr>
	</thead>
	<tbody>
		{foreach $connections as $entry}
		<tr>
			<td class="name"><a href="/admin/connection/{$entry.id}">{$entry.name}</a></td>
			<td class="dim">{$entry.driver}</td>
			<td>
				{if $entry.is_enabled == 0}<span class="dot"></span>Disabled
				{elseif $entry.status == "ok"}<span class="dot dot--ok"></span>OK
				{elseif $entry.status == "error"}<span class="dot dot--bad"></span>Error
				{else}<span class="dot"></span>Not tested{/if}
			</td>
			<td class="num c-medium">{$entry.table_count}</td>
			<td class="num c-wide">{$entry.grants}</td>
			<td class="flex c-wide"><code class="truncate">{$entry.dsn}</code></td>
			<td class="actions"><a href="/admin/connection/{$entry.id}">Edit</a></td>
		</tr>
		{/foreach}
	</tbody>
</table>
</div>
{else}
<p class="empty-state"><b>No connections</b>Add one below and dbadmin will be able to browse it.</p>
{/if}

<h2>Add a connection</h2>

<form method="post" action="/admin/connection" class="stack">
	<input type="hidden" name="csrf_token" value="{$ctx.csrf_token}">

	<div class="field">
		<label for="c-name">Name</label>
		<input id="c-name" name="name" value="{$name}" autocomplete="off" required>
		<small>Lowercase letters, digits, dash or underscore. This is the name the runtime registers.</small>
	</div>
	<div class="field grow">
		<label for="c-dsn">DSN</label>
		<input id="c-dsn" name="dsn" value="{$dsn}" autocomplete="off" required
			placeholder="postgres://user:pass@host:5432/database?sslmode=disable">
		<small>sqlite://path.db &middot; mysql://user:pass@tcp(host:3306)/database &middot; postgres://user:pass@host/database</small>
	</div>
	<div class="field">
		<label for="c-schema">Default schema</label>
		<input id="c-schema" name="schema" autocomplete="off" placeholder="public">
		<small>Ignored on sqlite. Defaults to <code>public</code> on postgres and to the DSN's database on mysql.</small>
	</div>
	<label class="check"><input type="checkbox" name="readonly" value="1"> Read-only for everyone</label>

	<button type="submit">Add connection</button>
</form>

<p class="note">The DSN is stored in dbadmin's own database in cleartext, because it has to be replayed to open the
connection. The file is restricted to its owner at startup and the DSN is redacted everywhere it is shown; a database
you would not put behind that is a database this should not hold the password for.</p>
