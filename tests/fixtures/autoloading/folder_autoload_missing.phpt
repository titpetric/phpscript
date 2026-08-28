name: autoload folder miss is an ordinary undefined class
description: >
  A class the folder does not hold is not special: construction throws the same
  catchable Error an undefined class always throws, and class_exists answers
  false having looked. This one is held to php, because a miss must not diverge
  from what php does with a class nothing declares.
---
<?php

try {
    new Fixture\Absent();
} catch (Error $error) {
    echo "undefined class\n";
}
echo class_exists("Fixture\\Absent") ? "loaded\n" : "not loaded\n";
?>
---
undefined class
not loaded
