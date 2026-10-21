{include header.tpl}
<section class="card formcard"><div class="cardhead"><div><h2>{$mode} row</h2><p>Select "Use NULL" to store a SQL NULL value.</p></div></div><form method="post" class="stack">
{foreach $columns as $column}
{eval $is_null = !isset($row[$column.name]) || $row[$column.name] === null}<div class="fieldrow"><label><span>{$column.name} <code>{$column.type}</code></span><textarea name="value_{$column.cid}" rows="2">{if !$is_null}{$row[$column.name]}{/if}</textarea></label><label class="check"><input type="checkbox" name="null_{$column.cid}" value="1" {if $is_null}checked{/if}> Use NULL</label></div>
{/foreach}<div class="formactions"><a class="button secondary" href="/table/{$table}">Cancel</a><button class="button">{$mode} row</button></div></form></section>
{include footer.tpl}
