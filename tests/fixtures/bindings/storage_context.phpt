name: storage context injection
runner:
  php: false
description: >
  The runtime's context.Context is auto-injected into the constructor (mirroring
  vuego's wrapContextFunc). A value placed on the context (the tenant) is
  captured at construction and observable through a Go method.
---
<?php
$storage = new Storage;
$tenant = $storage->tenant();
echo $tenant;
---
acme
