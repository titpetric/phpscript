<?php
$_v = $this->vars;
?>
<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title><?php echo h($_v["title"]); ?> · SQLite Admin</title><link rel="stylesheet" href="/assets/style.css"></head>
<body><header class="topbar"><a class="brand" href="/"><span class="logo">S</span><span>SQLite <b>Admin</b></span></a>
<nav><a href="/">Catalogue</a><a href="/table/create">Create table</a><a href="/sql">SQL console</a></nav></header>
<main class="shell"><div class="pagehead"><div><div class="eyebrow">Database administration</div><h1><?php echo h($_v["title"]); ?></h1></div></div>
