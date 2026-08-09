<?php

<div class="toolbar"><form method="get" class="search"><input name="q" value="
echo h($search);
" placeholder="Search all columns"><button>Search</button></form><div><a class="button secondary" href="/table/
echo h($table);
/structure">Structure</a> <a class="button secondary" href="/table/
echo h($table);
/export">Export CSV</a> <a class="button" href="/table/
echo h($table);
/insert">+ Insert row</a></div></div>
if ($without_rowid) {
<div class="notice warning">This is a WITHOUT ROWID table. Browsing works, but row edit and delete actions require SQLite rowid and are unavailable.</div>
}
<section class="card"><div class="cardhead"><div><h2>
echo h($table);
</h2><p>
echo $total;
 matching rows · page 
echo $page;
</p></div></div><div class="tablewrap"><table><thead><tr>
if (!$without_rowid) {
<th>rowid</th>
}
foreach ($columns as $column) {
<th>
	echo h($column["name"]);
</th>
}
<th>Actions</th></tr></thead><tbody>
foreach ($rows as $row) {
<tr>
	if (!$without_rowid) {
<td><code>
		echo h($row["_rowid_"]);
</code></td>
	}
	foreach ($columns as $column) {
<td>
		echo h($row[$column["name"]]);
</td>
	}
<td class="actions">
	if (!$without_rowid) {
<a href="/table/
		echo h($table);
/row/
		echo h($row["_rowid_"]);
/edit">Edit</a><form method="post" action="/table/
		echo h($table);
/row/
		echo h($row["_rowid_"]);
/delete" onsubmit="return confirm('Delete this row?')"><button class="link danger">Delete</button></form>
	}
</td></tr>
}
if (count($rows) == 0) {
<tr><td colspan="99" class="empty">No rows found.</td></tr>
}
</tbody></table></div></section>
<div class="pagination">
if ($page > 1) {
<a href="?q=
	echo h($search);
&page=
	echo $page - 1;
">← Previous</a>
}
<span>Page 
echo $page;
</span>
if ($page * $limit < $total) {
<a href="?q=
	echo h($search);
&page=
	echo $page + 1;
">Next →</a>
}
</div>
