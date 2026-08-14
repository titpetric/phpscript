name: runtime introspection functions
description: >
  Runtime symbol APIs expose constants, internal and user functions, local
  variables, and both PHP and host-backed classes.
---
<?php

function fixture_introspection_function()
{
    return true;
}

function fixture_introspection_vars($argument)
{
    $local_variable = " local";
    $defined = get_defined_vars();
    return $defined["argument"] . $defined["local_variable"];
}

class FixtureIntrospectionClass
{
}

$local = "visible";
$constants = get_defined_constants();
$grouped_constants = get_defined_constants(true);
$functions = get_defined_functions();
$variables = get_defined_vars();
$classes = get_declared_classes();

echo isset($constants["PATH_SEPARATOR"]) ? "constant\n" : "missing constant\n";
echo isset($grouped_constants["Core"]["PATH_SEPARATOR"]) ? "grouped constant\n" : "missing grouped constant\n";
echo in_array("get_defined_vars", $functions["internal"]) ? "internal function\n" : "missing internal function\n";
echo in_array("fixture_introspection_function", $functions["user"]) ? "user function\n" : "missing user function\n";
echo $variables["local"] . "\n";
echo fixture_introspection_vars("function") . "\n";
echo in_array("FixtureIntrospectionClass", $classes) ? "php class\n" : "missing php class\n";
echo in_array("Exception", $classes) ? "host class\n" : "missing host class\n";
echo in_array("Database", $classes) ? "database host class\n" : "missing database host class\n";
?>
---
constant
grouped constant
internal function
user function
visible
function local
php class
host class
database host class
