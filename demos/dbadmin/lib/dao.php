<?php

/**
 * The DAO manifest.
 *
 * spl_autoload_register() exists in this runtime but `new $class` does not, so
 * an autoloader could find a file and then not be able to build anything out
 * of it. A flat list is the honest version of the same thing, and it makes the
 * dependency order visible: leaves first, so a class is defined before the one
 * that holds it is.
 *
 * require_once rather than require, because more than one entry point includes
 * this and the second one must not redeclare.
 */

require_once "modules/driver_dao.php";
require_once "modules/audit_dao.php";

require_once "modules/user_dao.php";
require_once "modules/session_dao.php";
require_once "modules/user_group_dao.php";
require_once "modules/connection_dao.php";
require_once "modules/acl_dao.php";

require_once "modules/tables_dao.php";
require_once "modules/browse_dao.php";
require_once "modules/row_dao.php";
require_once "modules/ddl_dao.php";
require_once "modules/sql_dao.php";

require_once "modules/login_dao.php";
require_once "modules/register_dao.php";
