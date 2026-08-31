name: a command starts where the script is
description: >
  Running a command leaves the source filesystem behind — a process reads the
  host with the permissions of the user running the server — but it starts in
  the directory the script is in, so a relative path means the same thing to the
  command as it does to the script that ran it. chdir moves both. The assertion
  is a basename because php answers pwd with a host path and phpscript's root is
  the fixture directory, which is the one divergence here.
---
<?php

echo trim(shell_exec("basename \"$(pwd)\"")), "|";
chdir("tree");
echo trim(shell_exec("basename \"$(pwd)\"")), "|";
echo trim(shell_exec("cat marker.txt")), "|";
echo trim(exec("cat marker.txt")), "|";
chdir("..");
echo trim(shell_exec("basename \"$(pwd)\"")), "\n";
?>
---
pexec|tree|in tree|in tree|pexec
