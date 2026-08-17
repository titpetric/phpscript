<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{title|escape}</title>
<link rel="stylesheet" href="/assets/style.css">
</head>
<body>
<main>
<h1>{title|escape}</h1>

{if $message}
<p class="notice">{message|escape}</p>
{/if}

<form method="post" action="/bookmarks">
	<input name="title" placeholder="Title" required>
	<input name="url" placeholder="https://example.com" required>
	<button>Add bookmark</button>
</form>

<ul class="bookmarks">
{foreach $bookmarks as $bookmark}
	<li>
		<a href="{bookmark.url|escape}">{bookmark.title|escape}</a>
		<small>{bookmark.created_at|escape}</small>
		<form method="post" action="/bookmarks/{bookmark.id}/delete">
			<button class="link">Delete</button>
		</form>
	</li>
{/foreach}
{if count($bookmarks) == 0}
	<li class="empty">No bookmarks yet.</li>
{/if}
</ul>
</main>
</body>
</html>
