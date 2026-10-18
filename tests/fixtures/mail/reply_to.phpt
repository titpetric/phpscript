name: a Reply-To that is not one address is refused before anything is sent
description: >
  `send()` takes an optional $html, sent beside $body as the alternative a mail
  client prefers, and an optional $reply_to. CR and LF are stripped from every
  header value, so a header smuggled into $reply_to behind a line break leaves
  one malformed address, and the call throws naming the argument rather than
  delivering it; a list is refused too, because the argument is one address.
  The refusal comes before the server is reached, which is why this fixture
  runs with no mail server to dial. The php runner is opted out: Mail is a
  host binding and the name does not exist in php.
runner:
  php: false
---
<?php

$mail = new Mail;

$replies = array(
	"lead@example.test\r\nBcc: everyone@example.test",
	"Team Lead <lead@example.test>\nBcc: everyone@example.test",
	"lead@example.test, everyone@example.test",
);
foreach ($replies as $reply) {
	try {
		$mail->send("a@example.test", "Week sent back", "Text", "<p>HTML</p>", $reply);
		echo "sent\n";
	} catch (Exception $e) {
		echo $e->getMessage(), "\n";
	}
}
?>
---
Mail::send(): argument #5 ($reply_to) must be one email address: mail: expected single address, got ": everyone@example.test"
Mail::send(): argument #5 ($reply_to) must be one email address: mail: expected single address, got "Bcc: everyone@example.test"
Mail::send(): argument #5 ($reply_to) must be one email address: mail: expected single address, got ", everyone@example.test"
