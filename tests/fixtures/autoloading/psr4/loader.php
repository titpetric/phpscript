<?php

namespace Acme\Loader;

/**
 * A PSR-4 class loader in the shape a generated autoloader takes: a static
 * bootstrap that registers a bound method as the autoload callback, a static
 * property holding the prefix map, and a scope-isolated include closure.
 */
class ClassLoader
{
	private static $instance;
	private static $includeFile;

	private $prefixes = array();

	public static function bootstrap($prefix, $dir)
	{
		if (null !== self::$instance) {
			return self::$instance;
		}
		self::$includeFile = \Closure::bind(static function ($file) {
			include $file;
		}, null, null);
		self::$instance = new ClassLoader();
		self::$instance->addPrefix($prefix, $dir);
		self::$instance->register();
		return self::$instance;
	}

	public function addPrefix($prefix, $dir)
	{
		$this->prefixes[$prefix] = $dir;
	}

	public function register($prepend = false)
	{
		spl_autoload_register(array($this, "loadClass"), true, $prepend);
	}

	public function unregister()
	{
		spl_autoload_unregister(array($this, "loadClass"));
	}

	public function loadClass($class)
	{
		if ($file = $this->findFile($class)) {
			$includeFile = self::$includeFile;
			$includeFile($file);
			return true;
		}
		return null;
	}

	public function findFile($class)
	{
		$logical = strtr($class, "\\", "/") . ".php";
		$subPath = $class;
		while (false !== $lastPos = strrpos($subPath, "\\")) {
			$subPath = substr($subPath, 0, $lastPos);
			$search = $subPath . "\\";
			if (isset($this->prefixes[$search])) {
				$candidate = $this->prefixes[$search] . "/" . substr($logical, $lastPos + 1);
				if (file_exists($candidate)) {
					return $candidate;
				}
			}
		}
		return false;
	}
}
