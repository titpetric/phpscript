{* One error block, used by every form. *}{if $errors}
	<ul class="notice notice--bad" role="alert">
		{foreach $errors as $error}
		<li>{$error|h}</li>
		{/foreach}
	</ul>
	{/if}
