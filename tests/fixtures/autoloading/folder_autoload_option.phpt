name: autoload folder is named by an option
description: >
  The folder is one configuration key, not a hard-coded name. Pointing it at lib
  resolves App\Widget from lib/App/Widget.php and takes the default autoload/
  directory out of play, so the class living there no longer resolves. php has no
  equivalent convention.
options:
  autoload: lib
runner:
  php: false
---
<?php

$widget = new App\Widget();
echo $widget->label() . "\n";
echo App\Widget::KIND . "\n";
echo class_exists("Fixture\\Loaded") ? "default folder used\n" : "default folder not used\n";
?>
---
loaded from lib
widget
default folder not used
