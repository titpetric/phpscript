name: the image functions cannot reach a path outside the root
description: >
  Every filesystem binding anchors the path a script supplies inside the root it
  was bound to, and the image functions are no exception. An absolute path names
  the root rather than the host filesystem, which is the same spelling getcwd()
  answers with, so a host path resolves under the root and finds nothing.

  The expectation is phpscript's sandbox rather than php's behaviour, so the php
  runner is opted out: php has no such root and would read and write the host
  path. What defines the output is that an image read refuses exactly what
  file_get_contents refuses, which is what the shared root is for. gd used to
  return an absolute path unchanged, so imagecreatefrompng read any file the
  process could reach and imagepng wrote one.
runner:
  php: false
---
<?php

// Reached relative to this fixture's folder, which is its root.
var_dump(imagecreatefrompng("testdata/portrait.png") !== false);

// A host path a tenant must not reach. Both bindings refuse it alike, because
// it resolves under the root where there is no such file.
var_dump(@imagecreatefrompng("/etc/hostname"));
var_dump(@file_get_contents("/etc/hostname"));

// A write named absolutely lands inside the root, not at the host path.
$im = imagecreatetruecolor(2, 2);
var_dump(@imagepng($im, "/clamped.png"));
var_dump(file_exists("/clamped.png"));
var_dump(file_exists("clamped.png"));
unlink("clamped.png");
var_dump(file_exists("clamped.png"));
---
bool(true)
bool(false)
bool(false)
bool(true)
bool(true)
bool(true)
bool(false)
