name: a catch clause filters on the class name
runner:
  php: false
description: >
  Every throwable class is one type carrying the name a script constructed, and
  a clause matches on that name: Exception takes anything not named *Error,
  Error takes anything that is, and any other name is compared exactly. There is
  no hierarchy, so a clause naming a parent does not take a child. PHP does
  filter by hierarchy, so only phpscript defines the expected output; see
  docs/design.md.
---
<?php

// One line per thrown class: which of the clause types below take it.
// A clause that does not match falls through to the Throwable clause.

$row = "Exception:";
try { throw new Exception("x"); } catch (Exception $e) { $row = $row . " Exception"; } catch (Throwable $e) { }
try { throw new Exception("x"); } catch (Error $e) { $row = $row . " Error"; } catch (Throwable $e) { }
try { throw new Exception("x"); } catch (RuntimeException $e) { $row = $row . " RuntimeException"; } catch (Throwable $e) { }
echo $row, "\n";

$row = "RuntimeException:";
try { throw new RuntimeException("x"); } catch (Exception $e) { $row = $row . " Exception"; } catch (Throwable $e) { }
try { throw new RuntimeException("x"); } catch (Error $e) { $row = $row . " Error"; } catch (Throwable $e) { }
try { throw new RuntimeException("x"); } catch (RuntimeException $e) { $row = $row . " RuntimeException"; } catch (Throwable $e) { }
try { throw new RuntimeException("x"); } catch (LogicException $e) { $row = $row . " LogicException"; } catch (Throwable $e) { }
echo $row, "\n";

$row = "InvalidArgumentException:";
try { throw new InvalidArgumentException("x"); } catch (Exception $e) { $row = $row . " Exception"; } catch (Throwable $e) { }
try { throw new InvalidArgumentException("x"); } catch (InvalidArgumentException $e) { $row = $row . " InvalidArgumentException"; } catch (Throwable $e) { }
try { throw new InvalidArgumentException("x"); } catch (LogicException $e) { $row = $row . " LogicException"; } catch (Throwable $e) { }
echo $row, "\n";

$row = "Error:";
try { throw new Error("x"); } catch (Exception $e) { $row = $row . " Exception"; } catch (Throwable $e) { }
try { throw new Error("x"); } catch (Error $e) { $row = $row . " Error"; } catch (Throwable $e) { }
echo $row, "\n";

$row = "TypeError:";
try { throw new TypeError("x"); } catch (Exception $e) { $row = $row . " Exception"; } catch (Throwable $e) { }
try { throw new TypeError("x"); } catch (Error $e) { $row = $row . " Error"; } catch (Throwable $e) { }
try { throw new TypeError("x"); } catch (TypeError $e) { $row = $row . " TypeError"; } catch (Throwable $e) { }
echo $row, "\n";

$row = "ArgumentCountError:";
try { throw new ArgumentCountError("x"); } catch (Error $e) { $row = $row . " Error"; } catch (Throwable $e) { }
try { throw new ArgumentCountError("x"); } catch (TypeError $e) { $row = $row . " TypeError"; } catch (Throwable $e) { }
echo $row, "\n";

echo "class=", get_class(new InvalidArgumentException("x")), "\n";
echo "code=", (new InvalidArgumentException("x", 7))->getCode(), "\n";
echo "message=", (new InvalidArgumentException("boom"))->getMessage(), "\n";
echo "parent=", var_export(get_parent_class(new InvalidArgumentException("x")), true), "\n";
---
Exception: Exception
RuntimeException: Exception RuntimeException
InvalidArgumentException: Exception InvalidArgumentException
Error: Error
TypeError: Error TypeError
ArgumentCountError: Error
class=InvalidArgumentException
code=7
message=boom
parent=false
