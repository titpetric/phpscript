{include header.tpl}
<section class="card formcard"><div class="cardhead"><div><h2>Sign in</h2><p>Credentials are checked against the application's .htpasswd file.</p></div></div>
{if $message != ""}<div class="notice">{$message}</div>{/if}
{if $configured}
<form method="post" class="stack"><label>Username<input name="username" autocomplete="username" required></label><label>Password<input type="password" name="password" autocomplete="current-password" required></label><div class="formactions"><button class="button">Sign in</button></div></form>
{/if}
</section>
{include footer.tpl}
