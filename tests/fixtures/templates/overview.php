<?php echo $this->getVar("title"); ?>

<?php foreach ($this->getVar("tables") as $name => $rows) {
	echo $name, ": ", $rows, " row(s)\n";
} ?>
