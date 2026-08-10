<?php
$_v = $this->vars;
include "templates/header.tpl";
?>
<section class="card formcard"><div class="cardhead"><div><h2>Run SQL</h2><p>Statements run directly against this local database.</p></div></div><form method="post" class="stack"><textarea class="query" name="query" rows="8" required><?php echo h($_v["query"]); ?></textarea><div class="formactions"><button class="button">Execute query</button></div></form></section>
<?php if ($_v["message"] != "") { ?><div class="notice success"><?php echo h($_v["message"]); ?></div><?php } if (count($_v["rows"]) > 0) { ?>
<section class="card"><div class="cardhead"><h2>Result</h2><span><?php echo count($_v["rows"]); ?> rows displayed</span></div><div class="tablewrap"><table><thead><tr><?php foreach ($_v["result_columns"] as $column) { ?><th><?php echo h($column); ?></th><?php } ?></tr></thead><tbody><?php foreach ($_v["rows"] as $row) { ?><tr><?php foreach ($_v["result_columns"] as $column) { ?><td><?php echo h($row[$column]); ?></td><?php } ?></tr><?php } ?></tbody></table></div></section>
<?php } include "templates/footer.tpl"; ?>
