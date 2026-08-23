<div class="pane-head">
	<div>
		<div class="eyebrow"><a href="/admin/connection">Connections</a></div>
		<h1>Connection test</h1>
	</div>
	<div class="pane-head__actions"><a href="/admin/connection/test">Run again</a></div>
</div>

<p class="lede">Every connection was opened read-only, probed for its version and asked for three counts. What each
column means depends on the driver: sqlite has one schema by definition, and mysql and postgres report the schemas this
login can actually see.</p>

{if $report}
<div class="scroll">
<table>
	<thead>
		<tr>
			<th class="name">Connection</th>
			<th class="flex">Status</th>
			<th class="num">Tables</th>
			<th class="num">Columns</th>
			<th class="num">Schemas</th>
			<th class="c-medium">Driver</th>
			<th class="c-wide">DSN</th>
		</tr>
	</thead>
	<tbody>
		{foreach $report as $entry}
		<tr{if $entry.status == "error"} class="failed"{/if}>
			<td class="name"><a href="/admin/connection/{$entry.id}">{$entry.name|h}</a></td>
			<td class="flex">
				{if $entry.status == "ok"}<span class="dot dot--ok"></span>OK
				{else}<span class="dot dot--bad"></span>{$entry.message|h}{/if}
			</td>
			<td class="num">{if $entry.status == "ok"}{$entry.tables}{else}&mdash;{/if}</td>
			<td class="num">{if $entry.status == "ok"}{$entry.columns}{else}&mdash;{/if}</td>
			<td class="num">{if $entry.status == "ok"}{$entry.schemas}{else}&mdash;{/if}</td>
			<td class="dim c-medium">{$entry.driver|h}</td>
			<td class="c-wide"><code class="truncate">{$entry.dsn|h}</code></td>
		</tr>
		{/foreach}
	</tbody>
</table>
</div>
{else}
<p class="empty-state"><b>Nothing to test</b><a href="/admin/connection">Add a connection</a> first.</p>
{/if}
