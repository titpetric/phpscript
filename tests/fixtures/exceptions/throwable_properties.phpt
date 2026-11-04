name: a throwable publishes no property
description: >
  php declares a throwable's message, code, file and line protected, so a
  script outside the class reads none of them back: get_object_vars answers
  the empty array and property_exists answers false. Every throwable here is
  one Go type carrying the message, the code and the class a catch filters on,
  and this pins that those fields stay out of both answers.
---
<?php

try {
    throw new InvalidArgumentException("boom", 2);
} catch (Exception $e) {
    var_dump(get_object_vars($e));
    var_dump(property_exists($e, "Message"));
    var_dump(property_exists($e, "Class"));
    echo $e->getMessage(), "/", $e->getCode(), "\n";
}
---
array(0) {
}
bool(false)
bool(false)
boom/2
