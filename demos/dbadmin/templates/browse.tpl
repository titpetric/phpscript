<?php
$_v = $this->vars;
include "templates/header.tpl";
?>
<div class="toolbar"><form method="get" class="search"><input name="q" value="<?php echo h($_v["search"]); ?>" placeholder="Search all columns"><button>Search</button></form><div><a class="button secondary" href="/table/<?php echo h($_v["table"]); ?>/structure">Structure</a> <a class="button secondary" href="/table/<?php echo h($_v["table"]); ?>/export">Export CSV</a> <a class="button" href="/table/<?php echo h($_v["table"]); ?>/insert">+ Insert row</a></div></div>
<?php if ($_v["without_rowid"]) { ?><div class="notice warning">This WITHOUT ROWID table can be browsed, but rows cannot be edited or deleted here.</div><?php } ?>
<section class="card"><div class="cardhead"><div><h2><?php echo h($_v["table"]); ?></h2><p><?php echo $_v["total"]; ?> matching rows · page <?php echo $_v["page"]; ?></p></div></div><div class="tablewrap"><table><thead><tr>
<?php if (!$_v["without_rowid"]) { ?><th>rowid</th><?php } foreach ($_v["columns"] as $column) { ?><th><?php echo h($column["name"]); ?></th><?php } ?><th>Actions</th></tr></thead><tbody>
<?php foreach ($_v["rows"] as $row) { ?><tr><?php if (!$_v["without_rowid"]) { ?><td><code><?php echo h($row["_rowid_"]); ?></code></td><?php } foreach ($_v["columns"] as $column) { ?><td><?php echo h($row[$column["name"]]); ?></td><?php } ?><td class="actions"><?php if (!$_v["without_rowid"]) { ?><a href="/table/<?php echo h($_v["table"]); ?>/row/<?php echo h($row["_rowid_"]); ?>/edit">Edit</a><form method="post" action="/table/<?php echo h($_v["table"]); ?>/row/<?php echo h($row["_rowid_"]); ?>/delete" onsubmit="return confirm('Delete this row?')"><button class="link danger">Delete</button></form><?php } ?></td></tr><?php } ?>
<?php if (count($_v["rows"]) == 0) { ?><tr><td colspan="99" class="empty">No rows found.</td></tr><?php } ?>
</tbody></table></div></section><div class="pagination"><?php if ($_v["page"] > 1) { ?><a href="?q=<?php echo h($_v["search"]); ?>&page=<?php echo $_v["page"] - 1; ?>">← Previous</a><?php } ?><span>Page <?php echo $_v["page"]; ?></span><?php if ($_v["page"] * $_v["limit"] < $_v["total"]) { ?><a href="?q=<?php echo h($_v["search"]); ?>&page=<?php echo $_v["page"] + 1; ?>">Next →</a><?php } ?></div>
<?php include "templates/footer.tpl"; ?>
