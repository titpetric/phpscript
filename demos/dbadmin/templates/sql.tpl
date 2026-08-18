{include header.tpl}
<section class="card formcard"><div class="cardhead"><div><h2>Run SQL</h2><p>Statements run directly against this local database.</p></div></div><form method="post" class="stack"><textarea class="query" name="query" rows="8" required>{$query|h}</textarea><div class="formactions"><button class="button">Execute query</button></div></form></section>
{if $message != ""}<div class="notice success">{$message|h}</div>{/if}
{if count($rows) > 0}<section class="card"><div class="cardhead"><h2>Result</h2><span>{$rows|count} rows displayed</span></div><div class="tablewrap"><table><thead><tr>{foreach $result_columns as $column}<th>{$column|h}</th>{/foreach}</tr></thead><tbody>{foreach $rows as $row}<tr>{foreach $result_columns as $column}<td>{$row[$column]|h}</td>{/foreach}</tr>{/foreach}</tbody></table></div></section>
{/if}
{include footer.tpl}
