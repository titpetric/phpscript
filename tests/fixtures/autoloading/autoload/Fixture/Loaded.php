<?php

namespace Fixture;

class Loaded
{
    var $source;

    public function __construct($source)
    {
        $this->source = $source;
    }

    public function message()
    {
        return "loaded by " . $this->source;
    }
}
