{include header.tpl}
<div class="stats"><div class="stat"><span>Tables</span><strong>{$table_count}</strong><small>User-managed tables</small></div><div class="stat"><span>Columns</span><strong>{$column_count}</strong><small>Across the database</small></div><div class="stat"><span>Rows</span><strong>{$row_count}</strong><small>Stored records</small></div></div>
<section class="card"><div class="cardhead"><div><h2>Table catalogue</h2><p>Inspect schemas and manage records.</p></div><a class="button" href="/table/create">+ New table</a></div>
<div class="tablewrap"><table><thead><tr><th>Table</th><th>Columns</th><th>Rows</th><th>Definition</th><th>Actions</th></tr></thead><tbody>
{foreach $tables as $table}
<tr><td><a class="tablename" href="/table/{$table.name|h}">{$table.name|h}</a></td><td>{$table.columns}</td><td>{$table.rows}</td><td><code class="truncate">{$table.sql|h}</code></td><td class="actions"><a href="/table/{$table.name|h}">Browse</a><a href="/table/{$table.name|h}/structure">Structure</a></td></tr>
{/foreach}
</tbody></table></div></section>
{include footer.tpl}
