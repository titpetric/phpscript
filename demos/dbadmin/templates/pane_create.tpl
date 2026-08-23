<div class="pane-head">
	<div>
		<div class="eyebrow"><a href="/tables">{$ctx.connection_name|h}</a>{if $ctx.schema_name} &middot; {$ctx.schema_name|h}{/if}</div>
		<h1>Create table</h1>
	</div>
</div>

{include errors.tpl}

<form method="post" action="/table/create" class="stack">
	<input type="hidden" name="csrf_token" value="{$ctx.csrf_token|h}">
	<input type="hidden" name="columns" value="{$spec|count}">

	<div class="field">
		<label for="table-name">Table name</label>
		<input id="table-name" name="name" value="{$name|h}" autocomplete="off" autofocus required>
	</div>

	<div class="scroll">
	<table class="form-table">
		<thead>
			<tr>
				<th class="name">Column</th>
				<th>Type</th>
				<th>Not null</th>
				<th>Primary</th>
				<th>Auto</th>
			</tr>
		</thead>
		<tbody>
			{foreach $spec as $index => $column}
			<tr>
				<td class="name"><input name="name_{$index}" value="{$column.name|hv}" autocomplete="off" placeholder="column"></td>
				<td>
					<select name="type_{$index}">
						{foreach $types as $type}
						<option value="{$type|h}"{if $type == $column.type} selected{/if}>{$type|h}</option>
						{/foreach}
					</select>
				</td>
				<td><input type="checkbox" name="notnull_{$index}" value="1" aria-label="Not null"{if $column.not_null} checked{/if}></td>
				<td><input type="checkbox" name="primary_{$index}" value="1" aria-label="Primary key"{if $column.primary} checked{/if}></td>
				<td><input type="checkbox" name="auto_{$index}" value="1" aria-label="Auto increment"{if $column.autoincrement} checked{/if}></td>
			</tr>
			{/foreach}
		</tbody>
	</table>
	</div>

	<div class="form-actions">
		<button type="submit">Create table</button>
		<a class="link" href="/tables">Cancel</a>
	</div>
</form>

<p class="note">Types are chosen from the list because a type cannot be sent to the server as a parameter, and the three
drivers spell an auto-incrementing key three mutually exclusive ways. Ticking <b>Auto</b> writes whichever one this
connection uses and makes the column the primary key.</p>
