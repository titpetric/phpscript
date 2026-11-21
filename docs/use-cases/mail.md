# Mail bindings

`mail()` and `Mail` send through a mail server the host configured. A script names a server, never spells one and cannot read one back.

That split is the whole design. Connection settings are operator configuration, the same way database credentials are. The [`mail` block](../configuration.md#mail-servers) holds them. They reach a script only as a name it may construct against.

Both bindings are part of the standard library. A script includes nothing to reach them.

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

Use `mail()` when the application keeps a single server. Use `Mail` when it sends transactional and bulk mail through different relays, the usual reason to have two.

## HTML and a Reply-To

`send()` takes two more arguments, both optional: `$html`, sent beside `$body` as the alternative a mail client prefers, and `$reply_to`, the address a reply goes to in place of the server's `from`:

```php
<?php

$mail = new Mail;
$mail->send(
	"ana@example.com",
	"Your week was sent back",
	"Dejan sent your week back: the Tuesday entry has no project.\n\nhttps://time.example.com/week/42",
	'<p>Dejan sent your week back: the Tuesday entry has no project.</p><p><a href="https://time.example.com/week/42">Open the week</a></p>',
	"Dejan <dejan@example.com>"
);
```

The arguments decide the shape of the message:

| `$body` | `$html` | Sent as                                                          |
|---------|---------|------------------------------------------------------------------|
| set     | empty   | `text/plain`, the message `mail()` sends, unchanged              |
| set     | set     | `multipart/alternative`: the text part first, the HTML part last |
| empty   | set     | one `text/html` part                                             |

Both parts of a multipart message are UTF-8 and quoted-printable. A lone HTML part is as well. A long line or a non-ASCII character reaches the relay as 7-bit lines of 76 characters at most. Write the text part by hand. A text-only client, a screen reader set to plain text, a spam filter: each reads it. Stripping the tags from the HTML drops every link target.

`$reply_to` takes one address, with or without a display name. The binding strips CR and LF from it as from all headers. A header smuggled in behind a line break leaves one malformed address, and the call throws before delivery:

```php
try {
	$mail->send($to, $subject, $text, $html, "lead@example.com\r\nBcc: everyone@example.net");
} catch (Exception $e) {
	echo $e->getMessage();
	// Mail::send(): argument #5 ($reply_to) must be one email address: mail:
	// expected single address, got ": everyone@example.net"
}
```

A plain-text message with a Reply-To passes `""` for `$html`. `mail()` takes neither argument: it keeps PHP's three arguments. PHP's `$additional_headers` stays unsupported.

## Configuring the servers

Servers are a map in the application's `phpscript.yml`, keyed by the name a script names:

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

`port` defaults to 25, with 587 as the submission port. Authentication is PLAIN. Given both `username` and `password`, the relay authenticates. Given neither, a local mailhog accepts the unauthenticated connection. `from` may hold a display name, in which case the bare address becomes the envelope sender.

The full key reference, including `insecure` and the STARTTLS rules, is in [Mail servers](../configuration.md#mail-servers).

## Failure is loud

The constructor refuses a name nobody configured, before a script has composed a message:

```php
try {
	$mail = new Mail("transactional");
} catch (Exception $e) {
	echo $e->getMessage();
	// no configuration found for mail server: transactional
}
```

Failing at startup means a typo surfaces on the request that introduced it, ahead of the first delivery and of a job nobody watches.

Delivery throws too. Wrap `send()` when the request should survive an unreachable mail server:

```php
$mail = new Mail;
try {
	$mail->send($address, "Reset your password", $link);
} catch (Exception $e) {
	// The user still needs the link; log it and tell them to retry.
	error_log("mail: " . $e->getMessage());
}
```

Here `mail()` diverges from PHP, which answers `false` and reports no reason. The binding queues no message and retries no delivery. A delivery either happened or threw, leaving the response to the application.

## What a script cannot do

A request holding a mail handle cannot leak the credential behind it:

```php
$mail = new Mail("marketing");   // configured with a username and password

echo json_encode($mail);         // {}
var_dump(get_object_vars($mail)); // array(0) {}
var_dump($mail->password);       // NULL
var_dump($mail->host);           // NULL
```

The object holds no properties. A property read, `var_dump`, `print_r`, `var_export`, `get_object_vars` and `json_encode` all find nothing. No call lists the configured servers either, because naming what exists is itself a disclosure. The settings are a configuration key, absent from `env`. `getenv()` never reaches them.

The constructor refuses the settings in place of a name and accepts nothing quietly:

```php
new Mail(array("host" => "mail.example.com", "password" => "secret"));
// Mail(): argument #1 ($name) must be the name of a configured server, not
// the settings of one: connection settings are the host's, and belong in the
// mail block of its configuration
```

## One server per site

Under [virtual hosting](virtual-hosting.md), each site owns its `mail` block. A site that declares any server gets only the servers it named. It cannot reach a relay another site configured:

```yaml
# /srv/shop/phpscript.yml
mail:
  default:
    host: mail.shop.example
    from: orders@shop.example
```

A site that declares no `mail` block inherits the operator's servers. `mail:` with nothing beneath it means no servers at all, and the site says it sends none.

The map replaces wholesale. A single unnamed block used to merge. A site setting only `host` and `from` kept the operator's `username` and `password` while authenticating as the operator.

## Sending by something other than SMTP

SMTP is the default transport. The model is separate: an embedding host swaps it without touching name resolution or the rules above:

```go
provider := mail.NewProviderFunc(func(config mail.Config, recipient, subject, body string) error {
	return ses.Send(config.From, recipient, subject, body)
}, servers)
options.Mail = provider
```

A `Deliver` reads three parts of a plain-text message. A provider built on that interface refuses a message with an HTML part or a Reply-To and names why, never delivering the text alone. The same holds for a host's `model.MailProvider`, whose `Send` is plain text. It sends the rest once it also declares `SendMessage(ctx context.Context, message mail.Message) error`. `mail.Provider` and `mail.Memory` do.

`mail.NewMemory(names...)` is a provider that queues messages and sends nothing and tests and dry runs capture mail without a mail server. Naming no servers configures every name. Each queued `mail.Message` holds the HTML part and the Reply-To as the script sent them.

See [Go bindings](bindings.md), which covers host installation.
