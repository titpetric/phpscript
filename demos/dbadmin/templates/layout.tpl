{* The page chrome. Every page is this file with a different pane loaded into
   it, so the header, the sidebar and the footer are written once.

   {include} is a compile-time paste and {load} is a runtime one, which is why
   the pane arrives as a variable: the pane is chosen per request, the chrome
   is not. Clear templates/cache/ after editing anything included here. *}<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{$title} &middot; dbadmin</title>
<link rel="stylesheet" href="/assets/css/dbadmin.css">
<script defer src="/assets/js/dbadmin.js"></script>
</head>
<body>

<header class="site-head">
	<div class="wrap site-head__inner">
		<a class="brand" href="/"><span class="brand__dot" aria-hidden="true"></span>dbadmin</a>

		{if $ctx.logged_in}
		<nav class="site-nav" aria-label="Primary">
			{if $ctx.connection_id}
			<a href="/tables">Tables</a>
			<a href="/sql">SQL</a>
			{/if}
			{if $ctx.is_admin}
			<a href="/admin/connection">Connections</a>
			<a href="/admin/user">Users</a>
			<a href="/admin/group">Groups</a>
			<a href="/admin/audit">Audit</a>
			{/if}
		</nav>

		<div class="session">
			<span class="session__user">{$ctx.username}</span>
			<form method="post" action="/logout">
				<input type="hidden" name="csrf_token" value="{$ctx.csrf_token}">
				<button type="submit" class="link">Sign out</button>
			</form>
		</div>
		{/if}
	</div>
</header>

{if $ctx.flash}
<p class="notice notice--ok" role="status">{$ctx.flash}</p>
{/if}

{if $standalone}
<main class="solo">
{load $content}
</main>
{else}
<div class="split">
{include sidebar.tpl}
	<main class="pane">
{load $content}
	</main>
</div>
{/if}

<footer class="site-foot">
	<span>dbadmin</span>
	<span class="site-foot__note">a phpscript demo</span>
</footer>

</body>
</html>
