<?php

namespace Acme\Greeting;

class Formal
{
	const SALUTATION = "Good day";

	public function greet($name)
	{
		return self::SALUTATION . ", " . $name . ".";
	}
}
