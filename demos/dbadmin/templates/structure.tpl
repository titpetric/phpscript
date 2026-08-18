{include header.tpl}
<div class="grid two"><section class="card"><div class="cardhead"><h2>Columns</h2><a class="button" href="/table/{$table|h}/insert">Insert row</a></div><div class="tablewrap"><table><thead><tr><th>Name</th><th>Type</th><th>Not null</th><th>Default</th><th>Primary key</th></tr></thead><tbody>
{foreach $columns as $column}<tr><td><b>{$column.name|h}</b></td><td><code>{$column.type|h}</code></td><td>{$column.notnull ? "Yes" : "No"}</td><td>{$column.dflt_value|h}</td><td>{$column.pk ? "Yes" : "No"}</td></tr>{/foreach}
</tbody></table></div></section><section class="card"><div class="cardhead"><h2>Indexes</h2></div><div class="tablewrap"><table><thead><tr><th>Name</th><th>Unique</th><th>Origin</th></tr></thead><tbody>
{foreach $indexes as $index}<tr><td>{$index.name|h}</td><td>{$index.unique ? "Yes" : "No"}</td><td>{$index.origin|h}</td></tr>{else}<tr><td colspan="3" class="empty">No indexes</td></tr>{/foreach}
</tbody></table></div></section></div><section class="card sqlcard"><div class="cardhead"><h2>CREATE statement</h2></div><pre>{$meta.sql|h}</pre></section>
<section class="dangerzone"><h2>Danger zone</h2><p>Permanently remove this table and all records.</p><form method="post" action="/table/{$table|h}/drop" onsubmit="return confirm('Drop this table?')"><label>Type <b>{$table|h}</b> to confirm<input name="confirmation" required></label><button class="button dangerbtn">Drop table</button></form></section>
{include footer.tpl}
