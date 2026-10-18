<?php

// Support file for chdir.phpt: reached as lib/greet.php only while the working
// directory is workdir, and chdir.phpt is what moves it there.
function workdir_greet()
{
	return "from lib";
}
