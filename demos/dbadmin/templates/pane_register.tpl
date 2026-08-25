<section class="card">
	<h1>Set up dbadmin</h1>
	<p class="lede">This installation has no accounts yet. The account you create now is the administrator: it can add
	connections, create other accounts and decide what each of them may do.</p>

	{include errors.tpl}

	<form method="post" action="/register" class="stack">
		<div class="field">
			<label for="username">Username</label>
			<input id="username" name="username" value="{$username}" autocomplete="username" autofocus required>
			<small>Letters, digits, dot, dash or underscore.</small>
		</div>
		<div class="field">
			<label for="password">Password</label>
			<input id="password" name="password" type="password" autocomplete="new-password" required>
			<small>At least 8 characters. bcrypt reads the first 72 bytes and ignores the rest.</small>
		</div>
		<div class="field">
			<label for="confirm">Repeat password</label>
			<input id="confirm" name="confirm" type="password" autocomplete="new-password" required>
		</div>
		<button type="submit">Create the administrator</button>
	</form>
</section>
