{* The destructive-mode switch.

   It is drawn only when the account's policy is 'toggle'. A policy of
   'allowed' has nothing to switch and 'denied' has nothing to offer, and in
   both cases a control that did nothing would be worse than no control. *}		{if $ctx.offers_toggle}
		<form class="danger-switch{if $ctx.can_destroy} danger-switch--on{/if}" method="post" action="/session/destructive">
			<input type="hidden" name="csrf_token" value="{$ctx.csrf_token|h}">
			<input type="hidden" name="back" value="{$back_path|h}">

			{if $ctx.can_destroy}
			<span class="danger-switch__state" data-expires="{$ctx.destructive_until|h}">Destructive mode on</span>
			<button type="submit" name="off" value="1">Leave</button>
			{else}
			<span class="danger-switch__state">Destructive actions off</span>
			<button type="submit" name="on" value="1">Enable for 15 min</button>
			{/if}
		</form>
		{else}
		<p class="danger-switch danger-switch--fixed">
			{if $ctx.can_destroy}
			<span class="danger-switch__state">Destructive actions allowed</span>
			{else}
			<span class="danger-switch__state">Destructive actions blocked by policy</span>
			{/if}
		</p>
		{/if}
