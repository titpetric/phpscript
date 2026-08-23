{* The left panel: the connection in view, the schema when the driver has more
   than one, and the tables as a menu. *}	<aside class="rail">

		<form class="rail__switch" method="post" action="/session/connection">
			<input type="hidden" name="csrf_token" value="{$ctx.csrf_token|h}">
			<label for="rail-connection">Connection</label>
			<select id="rail-connection" name="connection_id" data-submit>
				<option value="0">Choose a connection</option>
				{foreach $sidebar.connections as $option}
				<option value="{$option.id}"{if $option.id == $ctx.connection_id} selected{/if}>{$option.name|h}</option>
				{/foreach}
			</select>
			<noscript><button type="submit">Switch</button></noscript>
		</form>

		{if $sidebar.schemas}
		<form class="rail__switch" method="post" action="/session/schema">
			<input type="hidden" name="csrf_token" value="{$ctx.csrf_token|h}">
			<label for="rail-schema">Schema</label>
			<select id="rail-schema" name="schema" data-submit>
				{foreach $sidebar.schemas as $name}
				<option value="{$name|h}"{if $name == $ctx.schema_name} selected{/if}>{$name|h}</option>
				{/foreach}
			</select>
			<noscript><button type="submit">Switch</button></noscript>
		</form>
		{/if}

		{if $ctx.connection_id}
		<div class="rail__section">
			<label for="rail-filter">Tables <b>{$sidebar.tables|count}</b></label>
			<input id="rail-filter" type="search" placeholder="Filter" autocomplete="off" data-filter="#rail-tables">
		</div>

		{if $sidebar.error}
		<p class="rail__error"><span class="dot dot--bad"></span>{$sidebar.error|h}</p>
		{/if}

		<ul class="rail__tables" id="rail-tables">
			{foreach $sidebar.tables as $entry}
			<li><a href="/t/{$entry.name|urlencode}"{if $entry.name == $sidebar.active_table} class="active"{/if}>{$entry.name|h}</a></li>
			{else}
			<li class="rail__empty">No tables</li>
			{/foreach}
		</ul>

		<div class="rail__section">
			{if $ctx.is_readonly == false}<a class="rail__action" href="/table/create">+ New table</a>{/if}
			<a class="rail__action" href="/sql">SQL console</a>
		</div>
		{/if}

		{include destructive.tpl}
	</aside>
