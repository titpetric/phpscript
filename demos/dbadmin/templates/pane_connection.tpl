<div class="pane-head">
	<div>
		<div class="eyebrow"><a href="/admin/connection">Connections</a></div>
		<h1>{$connection.name}</h1>
	</div>
	<div class="pane-head__actions"><a href="/admin/connection/{$connection.id}?test=1">Test now</a></div>
</div>

{include errors.tpl}

{if $result}
	{if $result.status == "ok"}
	<p class="notice notice--ok"><span class="dot dot--ok"></span>Reachable: {$result.tables} table(s), {$result.columns}
	column(s), {$result.schemas} schema(s).</p>
	{else}
	<p class="notice notice--bad"><span class="dot dot--bad"></span>{$result.message}</p>
	{/if}
{/if}

<dl class="detail">
	<div><dt>Driver</dt><dd>{$connection.driver}</dd></div>
	<div><dt>DSN</dt><dd><code>{$redacted}</code></dd></div>
	<div><dt>Status</dt><dd>{$connection.status}{if $connection.status_message} &mdash; {$connection.status_message}{/if}</dd></div>
	<div><dt>Last checked</dt><dd>{$connection.checked_at}</dd></div>
	<div><dt>Created</dt><dd>{$connection.created_at}</dd></div>
</dl>

<h2>Settings</h2>

<form method="post" action="/admin/connection/{$connection.id}" class="stack">
	<input type="hidden" name="csrf_token" value="{$ctx.csrf_token}">

	<div class="field grow">
		<label for="c-dsn">DSN</label>
		<input id="c-dsn" name="dsn" value="{$connection.dsn}" autocomplete="off" required>
		<small>Shown in full here because this is the form that changes it.</small>
	</div>
	<div class="field">
		<label for="c-schema">Default schema</label>
		<input id="c-schema" name="schema" value="{$connection.default_schema}" autocomplete="off">
	</div>
	<label class="check"><input type="checkbox" name="enabled" value="1"{if $connection.is_enabled} checked{/if}> Enabled</label>
	<label class="check"><input type="checkbox" name="readonly" value="1"{if $connection.is_readonly} checked{/if}> Read-only for everyone</label>

	<button type="submit">Save</button>
</form>

<h2>Groups that reach it</h2>

{if $grants}
<div class="scroll">
<table>
	<thead><tr><th class="name flex">Group</th><th>Mode</th></tr></thead>
	<tbody>
		{foreach $grants as $grant}
		<tr>
			<td class="name flex"><a href="/admin/group/{$grant.id}">{$grant.name}</a></td>
			<td class="dim">{if $grant.is_readonly}read-only{else}read and write{/if}</td>
		</tr>
		{/foreach}
	</tbody>
</table>
</div>
{else}
<p class="note">No group reaches this connection, so only administrators can open it.</p>
{/if}

<h2>Danger zone</h2>

<div class="danger-zone">
	<form method="post" action="/admin/connection/{$connection.id}/delete" class="danger-form">
		<input type="hidden" name="csrf_token" value="{$ctx.csrf_token}">
		<div>
			<b>Delete this connection</b>
			<p>Removes it from dbadmin, along with every grant and every session pointing at it. The database itself is
			not touched.</p>
		</div>
		<div class="danger-form__confirm">
			<input name="confirmation" placeholder="Type {$connection.name} to confirm" autocomplete="off"
				data-confirm-name="{$connection.name}">
			<button type="submit" class="danger">Delete</button>
		</div>
	</form>
</div>
