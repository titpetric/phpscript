name: namespaced class modifiers
description: >
  A namespaced file declaring abstract, final, readonly and final readonly
  classes. The modifiers are parsed and printed back, but none of them is
  enforced: the point of the fixture is that a namespaced file carrying them
  loads at all, and that the classes it declares construct and answer their
  own methods afterwards.
---
<?php

require "modifiers/shapes.php";

use App\Shapes\Point;
use App\Shapes\Sealed;
use App\Shapes\Tag;
use App\Shapes\Unit;

$sealed = new Sealed("width");
echo $sealed->label(), "\n";

$point = new Point(3, 4);
echo $point->x, ",", $point->y, " sum=", $point->sum(), "\n";

$tag = new Tag("done");
echo $tag->shout(), "\n";

echo Unit::METERS, "/", Unit::FEET, "\n";
---
width (m)
3,4 sum=7
DONE
m/ft
