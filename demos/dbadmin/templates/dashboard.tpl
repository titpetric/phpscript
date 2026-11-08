{include header.tpl}
<div class="stats"><div class="stat"><span>Tables</span><strong>{$table_count}</strong></div><div class="stat"><span>Columns</span><strong>{$column_count}</strong></div><div class="stat"><span>Rows</span><strong>{$row_count}</strong></div></div>
<section class="card"><div class="cardhead"><h2>Table catalogue</h2><a class="button" href="/table/create" title="New table" aria-label="New table">+</a></div>
<div class="tablewrap"><table><thead><tr><th>Table</th><th>Columns</th><th>Rows</th><th>Definition</th><th>Actions</th></tr></thead><tbody>
{foreach $tables as $table}
<tr><td><a class="tablename" href="/table/{$table.name}">{$table.name}</a></td><td>{$table.columns}</td><td>{$table.rows}</td><td><code class="truncate">{$table.sql}</code></td><td class="actions"><a href="/table/{$table.name}" title="Browse" aria-label="Browse">&#9776;</a><a href="/table/{$table.name}/structure" title="Structure" aria-label="Structure">&#9881;</a></td></tr>
{/foreach}
</tbody></table></div></section>
{include footer.tpl}
