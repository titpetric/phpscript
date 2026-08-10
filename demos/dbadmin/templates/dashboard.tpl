<?php
$_v = $this->vars;
include "templates/header.tpl";
?>
<div class="stats"><div class="stat"><span>Tables</span><strong><?php echo $_v["table_count"]; ?></strong><small>User-managed tables</small></div><div class="stat"><span>Columns</span><strong><?php echo $_v["column_count"]; ?></strong><small>Across the database</small></div><div class="stat"><span>Rows</span><strong><?php echo $_v["row_count"]; ?></strong><small>Stored records</small></div></div>
<section class="card"><div class="cardhead"><div><h2>Table catalogue</h2><p>Inspect schemas and manage records.</p></div><a class="button" href="/table/create">+ New table</a></div>
<div class="tablewrap"><table><thead><tr><th>Table</th><th>Columns</th><th>Rows</th><th>Definition</th><th>Actions</th></tr></thead><tbody>
<?php foreach ($_v["tables"] as $table) { ?>
<tr><td><a class="tablename" href="/table/<?php echo h($table["name"]); ?>"><?php echo h($table["name"]); ?></a></td><td><?php echo $table["columns"]; ?></td><td><?php echo $table["rows"]; ?></td><td><code class="truncate"><?php echo h($table["sql"]); ?></code></td><td class="actions"><a href="/table/<?php echo h($table["name"]); ?>">Browse</a><a href="/table/<?php echo h($table["name"]); ?>/structure">Structure</a></td></tr>
<?php } ?>
</tbody></table></div></section>
<?php include "templates/footer.tpl"; ?>
