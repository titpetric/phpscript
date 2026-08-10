<?php
$_v = $this->vars;
include "templates/header.tpl";
?>
<section class="card formcard"><div class="cardhead"><div><h2><?php echo h($_v["mode"]); ?> row</h2><p>Select “Use NULL” to store a SQL NULL value.</p></div></div><form method="post" class="stack">
<?php foreach ($_v["columns"] as $column) { $name = $column["name"]; $value = ""; $is_null = true; if (isset($_v["row"][$name]) && $_v["row"][$name] !== null) { $value = $_v["row"][$name]; $is_null = false; } ?>
<div class="fieldrow"><label><span><?php echo h($name); ?> <code><?php echo h($column["type"]); ?></code></span><textarea name="value_<?php echo h($column["cid"]); ?>" rows="2"><?php echo h($value); ?></textarea></label><label class="check"><input type="checkbox" name="null_<?php echo h($column["cid"]); ?>" value="1" <?php if ($is_null) { ?>checked<?php } ?>> Use NULL</label></div>
<?php } ?><div class="formactions"><a class="button secondary" href="/table/<?php echo h($_v["table"]); ?>">Cancel</a><button class="button"><?php echo h($_v["mode"]); ?> row</button></div></form></section>
<?php include "templates/footer.tpl"; ?>
