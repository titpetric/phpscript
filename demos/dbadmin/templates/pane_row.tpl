{include table_head.tpl}

<h1 class="pane-title">{if $inserting}Insert a row{else}Edit a row{/if}</h1>

{include errors.tpl}

<form method="post" class="stack" action="{if $inserting}/t/{$table|urlencode}/insert{else}/t/{$table|urlencode}/row/{$key|urlencode}/edit{/if}">
	<input type="hidden" name="csrf_token" value="{$ctx.csrf_token}">

	<div class="scroll">
	<table class="form-table">
		<thead><tr><th class="name">Column</th><th class="c-medium">Type</th><th class="flex">Value</th><th>Null</th></tr></thead>
		<tbody>
			{foreach $columns as $column}
			<tr>
				<td class="name">{$column.name}{if $column.is_key}<span class="status">PK</span>{/if}</td>
				<td class="dim c-medium">{$column.type}</td>
				<td class="flex"><input name="f_{$column.name}" value="{$values[$column.name]}" autocomplete="off"></td>
				<td>{if $column.nullable}<input type="checkbox" name="null_{$column.name}" value="1" aria-label="Set null"{if $nulls[$column.name]} checked{/if}>{/if}</td>
			</tr>
			{/foreach}
		</tbody>
	</table>
	</div>

	<div class="form-actions">
		<button type="submit">{if $inserting}Insert{else}Save{/if}</button>
		<a class="link" href="/t/{$table|urlencode}">Cancel</a>
	</div>
</form>

<p class="note">A column left blank is written as the empty string. Tick <b>Null</b> to write SQL NULL instead; the two
are different values and a form cannot tell them apart on its own.</p>
