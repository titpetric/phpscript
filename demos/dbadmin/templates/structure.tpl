<?php
$_v = $this->vars;
include "templates/header.tpl";
?>
<div class="grid two"><section class="card"><div class="cardhead"><h2>Columns</h2><a class="button" href="/table/<?php echo h($_v["table"]); ?>/insert">Insert row</a></div><div class="tablewrap"><table><thead><tr><th>Name</th><th>Type</th><th>Not null</th><th>Default</th><th>Primary key</th></tr></thead><tbody>
<?php foreach ($_v["columns"] as $column) { ?><tr><td><b><?php echo h($column["name"]); ?></b></td><td><code><?php echo h($column["type"]); ?></code></td><td><?php echo $column["notnull"] ? "Yes" : "No"; ?></td><td><?php echo h($column["dflt_value"]); ?></td><td><?php echo $column["pk"] ? "Yes" : "No"; ?></td></tr><?php } ?>
</tbody></table></div></section><section class="card"><div class="cardhead"><h2>Indexes</h2></div><div class="tablewrap"><table><thead><tr><th>Name</th><th>Unique</th><th>Origin</th></tr></thead><tbody>
<?php foreach ($_v["indexes"] as $index) { ?><tr><td><?php echo h($index["name"]); ?></td><td><?php echo $index["unique"] ? "Yes" : "No"; ?></td><td><?php echo h($index["origin"]); ?></td></tr><?php } if (count($_v["indexes"]) == 0) { ?><tr><td colspan="3" class="empty">No indexes</td></tr><?php } ?>
</tbody></table></div></section></div><section class="card sqlcard"><div class="cardhead"><h2>CREATE statement</h2></div><pre><?php echo h($_v["meta"]["sql"]); ?></pre></section>
<section class="dangerzone"><h2>Danger zone</h2><p>Permanently remove this table and all records.</p><form method="post" action="/table/<?php echo h($_v["table"]); ?>/drop" onsubmit="return confirm('Drop this table?')"><label>Type <b><?php echo h($_v["table"]); ?></b> to confirm<input name="confirmation" required></label><button class="button dangerbtn">Drop table</button></form></section>
<?php include "templates/footer.tpl"; ?>
