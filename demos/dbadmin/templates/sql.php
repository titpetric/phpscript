<?php

<section class="card formcard"><div class="cardhead"><div><h2>Run SQL</h2><p>Administrative console: statements run directly against this database.</p></div></div><form method="post" class="stack"><textarea class="query" name="query" rows="8" required placeholder="SELECT * FROM catalogue;">
echo h($query);
</textarea><div class="formactions"><button class="button">Execute query</button></div></form></section>
if ($message != "") {
<div class="notice success">
	echo h($message);
</div>
}
if (count($rows) > 0) {
<section class="card"><div class="cardhead"><h2>Result</h2><span>
	echo count($rows);
 rows displayed</span></div><div class="tablewrap"><table><thead><tr>
	foreach ($result_columns as $column) {
<th>
		echo h($column);
</th>
	}
</tr></thead><tbody>
	foreach ($rows as $row) {
<tr>
		foreach ($result_columns as $column) {
<td>
			echo h($row[$column]);
</td>
		}
</tr>
	}
</tbody></table></div></section>
}
