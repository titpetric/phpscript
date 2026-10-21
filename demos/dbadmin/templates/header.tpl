{* An include is a compile-time textual paste, so this partial deliberately ends
   without a trailing newline; the newline after the include line in the calling
   template supplies it. Never write an include tag inside a comment here: the
   compiler expands includes before it strips comments, so the partial would
   recursively include itself. *}<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{$title} · SQLite Admin</title><link rel="stylesheet" href="/assets/style.css"></head>
<body><header class="topbar"><a class="brand" href="/"><span class="logo">S</span><span>SQLite <b>Admin</b></span></a>
<nav><a href="/">Catalogue</a><a href="/table/create">Create table</a><a href="/sql">SQL console</a></nav>
{if $username != ""}<form class="signedin" method="post" action="/logout"><span class="who">{$username}</span><button class="linkbutton">Sign out</button></form>{/if}</header>
<main class="shell"><div class="pagehead"><div><div class="eyebrow">Database administration</div><h1>{$title}</h1></div></div>