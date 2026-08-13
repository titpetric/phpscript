---
name: in-process telemetry span ring buffer
description: Exercises PS\Telemetry bridge for in-process span measurements and ring buffer recording.
---
<?php

$span1 = new PS\Telemetry("database_query", "db");
$span1->close();

$span2 = new PS\Telemetry("template_render", "view");
$span2->end();

echo "telemetry recorded successfully\n";
?>
---
telemetry recorded successfully
