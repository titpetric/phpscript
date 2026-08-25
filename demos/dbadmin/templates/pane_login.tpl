<section class="card">
	<h1>Sign in</h1>

	{include errors.tpl}

	<form method="post" action="/login" class="stack">
		<div class="field">
			<label for="username">Username</label>
			<input id="username" name="username" value="{$username}" autocomplete="username" autofocus required>
		</div>
		<div class="field">
			<label for="password">Password</label>
			<input id="password" name="password" type="password" autocomplete="current-password" required>
		</div>
		<button type="submit">Sign in</button>
	</form>
</section>
