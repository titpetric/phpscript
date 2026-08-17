<?php

include("./Compiler.php");
include("./Template.php");

include("./TestCase.php");

class TemplateTest
{
	public $assert;

	function __construct() {
		$this->testcase = new TestCase;
	}

	function run() {
		// needs: ob_start, ob_get_contents, ob_end_clean
		// $this->testRendering();

		$this->testGetVar();
		$this->testException();
		$this->testFailure();

		$templates = $this->compileProvider();
		foreach $templates as $template {
			$this->testCompile($template);
		}

		$vars = $this->varsProvider();
		foreach $vars as $args {
			call_user_func_array($this->testVars, $args);
		}

		echo sprintf("Tests passing %d assertions\n", $this->testcase->assertions);
	}

	/**
	 * @dataProvider compileProvider
	 */
	#[\PHPUnit\Framework\Attributes\DataProvider('compileProvider')]
	public function testCompile($template)
	{
		$destination = "compiled/".$template;
		$compiled = "compiled/".$template;

		$tpl = new MiniTPL\Template;

		$tpl->set_paths("templates/");
		$tpl->set_compile_location("compiled/", false);
		$tpl->add_default("key", "val");

		$source = "templates/".$template;
		$return = $tpl->compile($source, $destination);

		$this->testcase->assertTrue($return == 1);
		$this->testcase->assertFileEquals($destination, $compiled);
	}

	public static function compileProvider()
	{
		$tests = array();
		$templates = glob("templates/*.tpl");
		//sort($templates);
		foreach ($templates as $template) {
			$tests[] = basename($template);
		}
		return $tests;
	}

	public function testRendering()
	{
		$tpl = new MiniTPL\Template;

		$tpl->set_paths("templates/");
		$tpl->set_compile_location("compile/", false);
		$tpl->add_default("key", "val");

		$retval = array();

		$this->testcase->assertTrue($tpl->load("08_utf8_bom.tpl"));

		touch("templates/compile/08_utf8_bom.tpl", filemtime("templates/08_utf8_bom.tpl") - 86400);

		$this->testcase->assertTrue($tpl->load("08_utf8_bom.tpl"));

		$i = 0;
		$items = array();
		$items[] = array("id" => $i++);
		$items[] = array("id" => $i++);
		$items[] = array("id" => $i++);
		$tpl->assign("items", $items);
		$tpl->assign(array("foo"=>"bar", "d" => array("burger")), "foo");
		$tpl->assign(".foo_foo", "baz");
		$tpl->assign(".foo_d", array("steak", "beef", "pork", "chicken"));

		$contents1 = $tpl->get();

		ob_start();
		$tpl->render();
		$contents2 = ob_get_contents();
		ob_end_clean();

		$this->testcase->assertEquals($contents1, $contents2);

		$this->testcase->assertFalse($tpl->_find_path("404.tpl"));

		$tpl->set_compile_location("compile/", false);
		$this->testcase->assertEquals("templates/compile/", $tpl->_compile_path("templates/"));
		$tpl->set_compile_location("/test/compile/", true);
		$this->testcase->assertEquals("/test/compile/templates/", $tpl->_compile_path("templates/"));

		$retval = array();
		//exec("rm -rf templates/compile -rf", $retval);
	}

	public function testGetVar()
	{
		$tpl = new MiniTPL\Template;
		$tpl->assign("foo", "bar");
		$this->testcase->assertEquals("bar", $tpl->getVar("foo"));
	}

	public function testException()
	{
		$caught = false;
		try {
			$tpl = new MiniTPL\Template;
			$tpl->set_compile_location("compile/", false);
			$tpl->set_paths("templates2/");
			$tpl->load("fail_to_compile.tpl");
		} catch ($e) {
			$caught = true;
		}
		$this->testcase->assertTrue($caught);
	}

	/**
	 * @dataProvider varsProvider
	 */
	#[\PHPUnit\Framework\Attributes\DataProvider('varsProvider')]
	public function testVars($expression, $expected, $description)
	{
		$tpl = new MiniTPL\Compiler;
		$result = $tpl->_split_exp($expression);
		$this->testcase->assertEquals($expected, $result);
	}

	public function testFailure()
	{
		$caught = false;
		try {
			$tpl = new MiniTPL\Template;
			$this->testcase->assertFalse($tpl->load("missing.tpl"));
			$tpl->render();
		} catch ($e) {
			$caught = true;
		}
		$this->testcase->assertTrue($caught);
	}

	public static function varsProvider()
	{
		$vars = array();
		$vars[] = array('news_section_news_list.tpl', 'news_section_news_list.tpl', "normal string");
		$vars[] = array('$var', "\$_v['var']", "variable");
		$vars[] = array('$var.netko', "\$_v['var']['netko']", "array index");
		$vars[] = array('$var . "netko"', "\$_v['var'] . \"netko\"", "string concat");
		$vars[] = array('$var1 . $var2', "\$_v['var1'] . \$_v['var2']", "variable concat");
		$vars[] = array('$var1.$var2', "\$_v['var1'][\$_v['var2']]", "array var index");
		$vars[] = array('$items.0', "\$_v['items']['0']", "array int index");
		$vars[] = array('$tpl->get()', "\$_v['tpl']->get()", "object function");
		$vars[] = array('$tplx->get()', "\$_v['tplx']->get()", "object function");
		return $vars;
	}
}

$test = new TemplateTest;
$test->run();
