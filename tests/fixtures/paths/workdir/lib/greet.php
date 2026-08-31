<?php

// Support file for chdir.phpt: reached as lib/greet.php only while the working
// directory is workdir, which is the whole point of the fixture.
function workdir_greet()
{
	return "from lib";
}
