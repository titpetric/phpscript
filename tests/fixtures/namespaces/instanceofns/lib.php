<?php

namespace Foo;

class Bar
{
    public $x = 1;
}

function check()
{
    $b = new Bar();
    // The fully qualified spelling and the name relative to the current
    // namespace answer the same check.
    var_dump($b instanceof \Foo\Bar);
    var_dump($b instanceof Bar);
}
