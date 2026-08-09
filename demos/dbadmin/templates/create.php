<?php

<section class="card formcard"><div class="cardhead"><div><h2>New database table</h2><p>Add as many columns as needed and choose types for your database dialect.</p></div></div><form method="post" class="stack" id="createTableForm"><div class="formgrid"><label>Table name<input name="table_name" required pattern="[A-Za-z_][A-Za-z0-9_]*" placeholder="inventory"></label><label>Database type<select name="engine" id="engineSelect" onchange="changeEngine()"><option value="sqlite" 
if ($engine == "sqlite") {
selected
}
>SQLite</option><option value="pgsql" 
if ($engine == "pgsql") {
selected
}
>PostgreSQL</option><option value="mysql" 
if ($engine == "mysql") {
selected
}
>MySQL</option></select></label></div><input type="hidden" name="column_count" id="columnCount" value="4"><div class="tablewrap"><table class="editor"><thead><tr><th>Column name</th><th>Type</th><th>Not null</th><th>Default SQL</th><th>Primary key</th><th></th></tr></thead><tbody id="columnRows">
for ($i = 1; $i <= 4; $i++) {
<tr data-column="
	echo $i;
"><td><input name="name_
	echo $i;
" 
	if ($i == 1) {
required
	}
 placeholder="
	echo $i == 1 ? "id" : "column_name";
"></td><td><select class="type-select" data-default="
	echo $i == 1 ? "INTEGER" : "TEXT";
" name="type_
	echo $i;
"></select></td><td><input type="checkbox" name="notnull_
	echo $i;
" value="1"></td><td><input name="default_
	echo $i;
" placeholder="NULL or CURRENT_TIMESTAMP"></td><td><input type="checkbox" name="pk_
	echo $i;
" value="1"></td><td><button type="button" class="remove-column" onclick="removeColumn(this)" title="Remove column">&times;</button></td></tr>
}
</tbody></table></div><div class="editor-actions"><button type="button" class="button secondary" onclick="addColumn()">+ Add column</button><span>Up to 100 columns</span></div><div class="formactions"><a href="/" class="button secondary">Cancel</a><button class="button">Create table</button></div></form></section>
<script>
var typeSets = {
    sqlite: ['INTEGER','TEXT','REAL','NUMERIC','BLOB','BOOLEAN','DATE','TIME','DATETIME','TIMESTAMP'],
    pgsql: ['SMALLINT','INTEGER','BIGINT','SERIAL','BIGSERIAL','NUMERIC','REAL','DOUBLE PRECISION','BOOLEAN','CHAR','VARCHAR','TEXT','DATE','TIME','TIMESTAMP','TIMESTAMPTZ','JSON','JSONB','BYTEA','UUID'],
    mysql: ['TINYINT','SMALLINT','MEDIUMINT','INT','BIGINT','DECIMAL','FLOAT','DOUBLE','BOOLEAN','CHAR','VARCHAR','TEXT','MEDIUMTEXT','LONGTEXT','DATE','TIME','DATETIME','TIMESTAMP','YEAR','JSON','BLOB']
};
var nextColumn = 4;

function fillTypes(select, selected) {
    var engine = document.getElementById('engineSelect').value;
    var types = typeSets[engine];
    select.innerHTML = '';
    for (var i = 0; i < types.length; i++) {
        var option = document.createElement('option');
        option.value = types[i]; option.textContent = types[i];
        if (types[i] === selected) { option.selected = true; }
        select.appendChild(option);
    }
}

function changeEngine() {
    var selects = document.querySelectorAll('.type-select');
    for (var i = 0; i < selects.length; i++) { fillTypes(selects[i], selects[i].value || selects[i].getAttribute('data-default')); }
}

function addColumn() {
    if (nextColumn >= 100) { return; }
    nextColumn++;
    document.getElementById('columnCount').value = nextColumn;
    var row = document.createElement('tr'); row.setAttribute('data-column', nextColumn);
    row.innerHTML = '<td><input name="name_' + nextColumn + '" placeholder="column_name"></td>' +
        '<td><select class="type-select" name="type_' + nextColumn + '"></select></td>' +
        '<td><input type="checkbox" name="notnull_' + nextColumn + '" value="1"></td>' +
        '<td><input name="default_' + nextColumn + '" placeholder="NULL or CURRENT_TIMESTAMP"></td>' +
        '<td><input type="checkbox" name="pk_' + nextColumn + '" value="1"></td>' +
        '<td><button type="button" class="remove-column" onclick="removeColumn(this)" title="Remove column">&times;</button></td>';
    document.getElementById('columnRows').appendChild(row);
    fillTypes(row.querySelector('.type-select'), 'TEXT');
    row.querySelector('input').focus();
}

function removeColumn(button) {
    var rows = document.querySelectorAll('#columnRows tr');
    if (rows.length > 1) { button.closest('tr').remove(); }
}

changeEngine();
</script>
