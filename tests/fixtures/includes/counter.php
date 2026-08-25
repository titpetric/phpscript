<?php

// Support file for once.phpt: it announces itself on every execution, so the
// number of lines it prints is how many times it ran.
echo "counter ran\n";

$counter_runs = isset($counter_runs) ? $counter_runs + 1 : 1;
