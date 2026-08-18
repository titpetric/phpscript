<?php
declare(strict_types=1);

namespace App;

use MiniTPL\Compiler;
use MiniTPL\Template as Renderer;
use function array_map;

class Runner {
	function run() {
		$compiler = new Compiler();
		return $compiler;
	}
}
