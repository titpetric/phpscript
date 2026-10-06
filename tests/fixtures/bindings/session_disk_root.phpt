name: session disk storage cannot create a directory outside the root
description: >
  Session\Storage\Disk took the $storage_path the script named and created it
  with os.MkdirAll exactly as given, so a script could make a directory anywhere
  the process could write and keep session files there, while file_exists() on
  the same path answered false. The path now resolves through the runtime's rule
  and is held to writable_paths, so it is refused before anything is created.

  Both spellings are refused here and neither directory appears: an absolute path
  names the root and lands outside the one writable entry, and a relative path
  outside that entry is refused on its own terms.

  The expectation is phpscript's sandbox rather than php's behaviour, so the php
  runner is opted out: php has no root to anchor to and no such class. What
  defines the output is that the constructor refuses exactly what a write to the
  same path would.
runner:
  php: false
options:
  writable_paths: ["data"]
---
<?php

try {
	new Session\Storage\Disk("/etc/phpscript-sessions");
	echo "constructed\n";
} catch (Throwable $e) {
	echo "refused\n";
}
var_dump(is_dir("/etc/phpscript-sessions"));

try {
	new Session\Storage\Disk("elsewhere");
	echo "constructed\n";
} catch (Throwable $e) {
	echo "refused\n";
}
var_dump(is_dir("elsewhere"));
---
refused
bool(false)
refused
bool(false)
