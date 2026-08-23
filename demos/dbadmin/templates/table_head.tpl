{* The heading and tab strip every table page shares. *}<div class="pane-head">
	<div>
		<div class="eyebrow"><a href="/tables">{$ctx.connection_name|h}</a>{if $ctx.schema_name} &middot; {$ctx.schema_name|h}{/if}</div>
		<h1>{$table|h}</h1>
	</div>
</div>

<nav class="tabs">
	<a href="/t/{$table|urlencode}"{if $tab == "browse"} class="active"{/if}>Browse</a>
	<a href="/t/{$table|urlencode}/structure"{if $tab == "structure"} class="active"{/if}>Structure</a>
	{if $is_readonly == false}
	<a href="/t/{$table|urlencode}/insert"{if $tab == "insert"} class="active"{/if}>Insert</a>
	{/if}
	<a href="/t/{$table|urlencode}/export">Export</a>
</nav>
