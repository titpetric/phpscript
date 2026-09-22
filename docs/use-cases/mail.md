# Mail bindings

`mail()` and `Mail` send through a mail server the host configured. A script names a server; it never spells one, and it cannot read one back.

That split is the whole design. Connection settings are operator configuration, the same way database credentials are, so they live in the [`mail` block](../configuration.md#mail-servers) and reach a script only as a name it may ask for.

Both bindings are part of the standard library. No PHP include is needed.

## Sending one message

`mail()` is PHP's own spelling and uses the server called `default`:

```php
<?php

mail("hello@example.com", "Subject", "Body");
```

`Mail` is the same delivery with the server named:

```php
<?php

$mail = new Mail;              // the "default" server
$mail->send("hello@example.com", "Subject", "Body");

$campaigns = new Mail("marketing");
$campaigns->send("list@example.com", "Newsletter", "Issue 1");
```

Use `mail()` when one server is all the application has, and `Mail` when it sends transactional and bulk mail through different relays, which is the usual reason to have two.

## Configuring the servers

Servers are a map in the application's `phpscript.yml`, keyed by the name a script asks for:

```yaml
mail:
  default:
    host: mail.example.com
    port: 587
    username: noreply@example.com
    password: secret
    from: Example <noreply@example.com>
  marketing:
    host: relay.example.net
    username: campaigns
    password: another-secret
    from: Marketing <marketing@example.com>
```

`port` defaults to 25, and a submission host usually wants 587. Authentication is PLAIN and is used when both `username` and `password` are set; neither set means no authentication, which is what a local mailhog wants. `from` may carry a display name, in which case the bare address is used as the envelope sender.

The full key reference, including `insecure` and the STARTTLS rules, is in [Mail servers](../configuration.md#mail-servers).

## Failure is loud

A name nobody configured is refused when the object is constructed, before a script has composed a message:

```php
try {
	$mail = new Mail("transactional");
} catch (Exception $e) {
	echo $e->getMessage();
	// no configuration found for mail server: transactional
}
```

Failing there rather than at the first delivery means a typo surfaces on the request that introduced it, not weeks later in a job nobody watches.

Delivery throws too, so wrap `send()` when the request should survive an unreachable mail server:

```php
$mail = new Mail;
try {
	$mail->send($address, "Reset your password", $link);
} catch (Exception $e) {
	// The user still needs the link; log it and tell them to retry.
	error_log("mail: " . $e->getMessage());
}
```

This is where `mail()` diverges from PHP, which answers `false` and says nothing about why. Nothing is queued and nothing is retried: a delivery either happened or it threw, and what to do about it belongs to the application.

## What a script cannot do

The binding is built so that a request holding a mail handle cannot leak the credential behind it:

```php
$mail = new Mail("marketing");   // configured with a username and password

echo json_encode($mail);         // {}
var_dump(get_object_vars($mail)); // array(0) {}
var_dump($mail->password);       // NULL
var_dump($mail->host);           // NULL
```

The object carries no properties, so a property read, `var_dump`, `print_r`, `var_export`, `get_object_vars` and `json_encode` all find nothing. There is no call that lists the configured servers either, because naming what exists is itself a disclosure. And because the settings are a configuration key rather than an `env` entry, `getenv()` was never in reach of them.

Passing the settings instead of a name is refused rather than quietly accepted:

```php
new Mail(array("host" => "mail.example.com", "password" => "secret"));
// Mail(): argument #1 ($name) must be the name of a configured server, not
// the settings of one: connection settings are the host's, and belong in the
// mail block of its configuration
```

## One server per site

Under [virtual hosting](virtual-hosting.md), each site's `mail` block is its own. A site that declares any server gets only the ones it named, so it cannot reach a relay another site configured:

```yaml
# /srv/shop/phpscript.yml
mail:
  default:
    host: mail.shop.example
    from: orders@shop.example
```

A site that declares no `mail` block inherits the operator's servers, and `mail:` with nothing under it means no servers at all, which is how a site says it sends none.

The map replaces rather than merges, which matters more than it looks: when this was a single unnamed block, a site setting only `host` and `from` kept the operator's `username` and `password` and authenticated as the operator.

## Sending by something other than SMTP

SMTP is the default transport, not the model. An embedding host swaps it without touching name resolution or the rules above:

```go
provider := mail.NewProviderFunc(servers, func(config mail.Config, recipient, subject, body string) error {
	return ses.Send(config.From, recipient, subject, body)
})
options.Mail = provider
```

`mail.NewMemory(names...)` is a provider that queues messages instead of delivering them, which is how tests and dry runs capture mail without a mail server. Naming no servers configures every name.

See [Go bindings](bindings.md) for how a host installs these.
