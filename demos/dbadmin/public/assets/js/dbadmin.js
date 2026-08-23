/* dbadmin — progressive enhancement only.
 *
 * Every feature below has a working answer with scripting off: the selects
 * have a submit button in a <noscript>, the confirmations are enforced on the
 * server, the filter box only hides rows the page already rendered, and the
 * destructive-mode window is checked in SQL rather than by the countdown.
 *
 * No dependencies, no build step, one file. */

(function () {
	"use strict";

	/* A select that submits its own form. The switcher in the sidebar is a
	   form with one control, so a separate button is a second click for
	   nothing; the <noscript> beside it is what happens without this. */
	function autoSubmit() {
		var selects = document.querySelectorAll("select[data-submit]");
		for (var i = 0; i < selects.length; i++) {
			selects[i].addEventListener("change", function (event) {
				if (event.target.form) {
					event.target.form.submit();
				}
			});
		}
	}

	/* Confirm before a form that says it needs confirming. The server checks
	   the same thing; this is so the click is not the last chance to think. */
	function confirmForms() {
		var forms = document.querySelectorAll("form[data-confirm]");
		for (var i = 0; i < forms.length; i++) {
			forms[i].addEventListener("submit", function (event) {
				if (!window.confirm(event.currentTarget.getAttribute("data-confirm"))) {
					event.preventDefault();
				}
			});
		}
	}

	/* Drop and empty ask for the table name to be typed. The button stays
	   disabled until it matches, so the mistake is caught before the request
	   rather than by a 400 after it. The server checks it too. */
	function typedConfirmations() {
		var inputs = document.querySelectorAll("input[data-confirm-name]");
		for (var i = 0; i < inputs.length; i++) {
			(function (input) {
				var button = input.parentNode.querySelector("button");
				if (!button) {
					return;
				}

				var expected = input.getAttribute("data-confirm-name");
				var wasDisabled = button.disabled;

				function sync() {
					button.disabled = wasDisabled || input.value !== expected;
				}

				sync();
				input.addEventListener("input", sync);
			})(inputs[i]);
		}
	}

	/* The sidebar table filter. It hides list items and nothing else, so a
	   filtered list is still the list the server sent. */
	function railFilter() {
		var boxes = document.querySelectorAll("input[data-filter]");
		for (var i = 0; i < boxes.length; i++) {
			(function (box) {
				var list = document.querySelector(box.getAttribute("data-filter"));
				if (!list) {
					return;
				}

				box.addEventListener("input", function () {
					var needle = box.value.toLowerCase();
					var items = list.children;
					for (var j = 0; j < items.length; j++) {
						var text = (items[j].textContent || "").toLowerCase();
						items[j].hidden = needle !== "" && text.indexOf(needle) === -1;
					}
				});
			})(boxes[i]);
		}
	}

	/* The destructive-mode countdown.
	 *
	 * The window is decided in SQL and expires whether or not this runs; the
	 * label is here so somebody who left the tab open can see how long they
	 * have left rather than finding out by being refused. */
	function destructiveCountdown() {
		var label = document.querySelector("[data-expires]");
		if (!label) {
			return;
		}

		var expires = Date.parse(label.getAttribute("data-expires"));
		if (isNaN(expires)) {
			return;
		}

		var base = label.textContent;

		function tick() {
			var left = Math.floor((expires - Date.now()) / 1000);
			if (left <= 0) {
				label.textContent = base + " — expired";
				window.clearInterval(timer);
				return;
			}

			var minutes = Math.floor(left / 60);
			var seconds = left % 60;
			label.textContent = base + " — " + minutes + ":" + (seconds < 10 ? "0" : "") + seconds;
		}

		var timer = window.setInterval(tick, 1000);
		tick();
	}

	/* "/" focuses the search box, the way a pager does. Skipped when the
	   caret is already in a field, or the shortcut would eat the character. */
	function searchShortcut() {
		var box = document.getElementById("dbadmin-filter") || document.getElementById("rail-filter");
		if (!box) {
			return;
		}

		document.addEventListener("keydown", function (event) {
			if (event.key !== "/" || event.metaKey || event.ctrlKey || event.altKey) {
				return;
			}

			var active = document.activeElement;
			var tag = active ? active.tagName : "";
			if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") {
				return;
			}

			event.preventDefault();
			box.focus();
		});
	}

	autoSubmit();
	confirmForms();
	typedConfirmations();
	railFilter();
	destructiveCountdown();
	searchShortcut();
})();
