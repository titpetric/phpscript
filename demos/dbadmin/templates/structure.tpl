{include header.tpl}
<div class="grid two"><section class="card"><div class="cardhead"><h2>Columns</h2><a class="button" href="/table/{$table}/insert" title="Insert row" aria-label="Insert row">+</a></div><div class="tablewrap"><table><thead><tr><th>Name</th><th>Type</th><th>Not null</th><th>Default</th><th>Primary key</th></tr></thead><tbody>
{foreach $columns as $column}<tr><td><b>{$column.name}</b></td><td><code>{$column.type}</code></td><td>{if $column.notnull}&#10003;{else}-{/if}</td><td>{if $column.dflt_value === null}<span class="null">NULL</span>{else}{$column.dflt_value}{/if}</td><td>{if $column.pk}&#10003;{else}-{/if}</td></tr>{/foreach}
</tbody></table></div></section><section class="card"><div class="cardhead"><h2>Indexes</h2></div><div class="tablewrap"><table><thead><tr><th>Name</th><th>Unique</th><th>Origin</th></tr></thead><tbody>
{foreach $indexes as $index}<tr><td>{$index.name}</td><td>{if $index.unique}&#10003;{else}-{/if}</td><td>{$index.origin}</td></tr>{else}<tr><td colspan="3" class="empty">No indexes</td></tr>{/foreach}
</tbody></table></div></section></div><section class="card sqlcard"><div class="cardhead"><h2>CREATE statement</h2></div><pre>{$meta.sql}</pre></section>
<section class="dangerzone"><h2>Danger zone</h2><form method="post" action="/table/{$table}/drop" onsubmit="return confirm('Drop table?')"><label>Confirm <b>{$table}</b><input name="confirmation" required></label><button class="button dangerbtn" title="Drop table" aria-label="Drop table">&times;</button></form></section>
{include footer.tpl}
