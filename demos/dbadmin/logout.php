<?php

// @route POST /logout

// POST, because signing somebody out is a change: a GET would let any page on
// the internet end a visitor's session with an <img> tag. The header's button
// posts here and the manager does both halves - it drops the stored session
// and expires the cookie on the way out.
// setup.php and not bootstrap.php: signing out of a session that has already
// expired is the same request as signing out of a live one, and the gate would
// answer it with a redirect to the page this one redirects to anyway.
include "setup.php";

// The sign-out form carries the same token pair the sign-in form does, so a
// cross-site post cannot end a visitor's session even from a client that
// sends the session cookie anyway.
if (!csrf_check()) {
	http_response_code(403);
	echo "The request did not carry the sign-out form's token.";
	exit();
}

$session->destroy();
redirect_to("/login");
