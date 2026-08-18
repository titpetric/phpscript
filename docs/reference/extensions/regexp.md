# Regular expressions

| PHP feature                           | Status                | Notes                                                                        |
|---------------------------------------|-----------------------|------------------------------------------------------------------------------|
| `preg_match`, `preg_match_all`        | Partial compatibility | `PREG_PATTERN_ORDER` only; no offset capture, no flags argument.             |
| `preg_replace`                        | Partial compatibility | String pattern, replacement and subject only; no arrays, no `$limit`.        |
| `preg_quote`                          | Compatibility         | Escapes the PCRE metacharacters plus an optional delimiter.                  |
| Backreferences, lookahead, lookbehind | Compatibility         | Compiled by a backtracking engine, with a one-second match timeout.          |
| `preg_split`, `preg_replace_callback` | Not implemented       | Absent from the runtime.                                                     |
| Atomic groups `(?>...)`               | Compatibility         | Compiled by the backtracking engine.                                         |
| Possessive quantifiers `a++`          | Not implemented       | Neither engine accepts them; the pattern fails to compile and never matches. |
| The `U` (ungreedy) modifier           | Partial compatibility | Honoured by RE2; dropped when a pattern also needs the backtracking engine.  |
| Named groups `(?<name>...)`           | Partial compatibility | The group matches and is numbered; `$matches["name"]` is not populated.      |

PHP's regular expressions are PCRE. Go's standard `regexp` is RE2, a different engine with a different bargain: RE2 guarantees a match runs in time linear in the length of the subject, and pays for that by leaving out every construct that would require backtracking. Two of those omissions show up in ordinary PHP code.

## Two engines

phpscript compiles each pattern with whichever engine can express it:

| Pattern contains                           | Engine                   |
|--------------------------------------------|--------------------------|
| nothing RE2 omits                          | `regexp` (RE2)           |
| `\1` to `\9`, `(?=`, `(?!`, `(?<=`, `(?<!` | `regexp2` (backtracking) |
| anything else RE2 rejects at compile time  | `regexp2` (backtracking) |

RE2 is preferred because it is faster and because a pattern it accepts cannot be made to backtrack catastrophically by hostile input. The fallback engine has no such guarantee, so a match it runs is bounded by a one-second timeout; a pattern that exceeds it reports no match rather than hanging the request.

Compilation happens once per pattern per runtime and is cached, so the choice of engine is not paid for on each call.

Which engine ran is not observable from PHP. The same `$matches` shape comes back either way, and a group that did not participate in the match is the empty string in both.

The behaviour this replaced was worse than a missing feature: a pattern RE2 could not compile silently reported "no match", so a template compiler that pairs `{block foo}` with `{/block}` through `\1` produced *wrong output* rather than an error.

## What still differs from PCRE

**Delimiters and modifiers.** A pattern is written PHP-style, `/body/flags`. Any of `( { [ <` may open it, with the matching bracket closing. The modifiers `s`, `m`, `i`, `x` and `U` are translated; anything else (`u`, `D`, `A`, `S`) is accepted and ignored. UTF-8 is not a mode either engine has to be told about: Go strings are UTF-8 already, so `u` is redundant rather than unsupported.

**Escape sequences.** `\_` and an escaped space are rewritten to the bare character, because both engines reject them as unknown classes where PCRE accepts them.

**Replacements.** `preg_replace` accepts both PHP spellings of a group reference in the replacement, `\1` and `$1`, and normalises them to `${1}` so that a reference followed by a digit, `\1` before a literal `0`, still names group 1.

**Return shapes.** `preg_match` fills `$matches` with the first match's groups. `preg_match_all` fills it in `PREG_PATTERN_ORDER`: `$matches[0]` is every whole match, `$matches[1]` every capture of group 1, and so on. Neither accepts the flags argument that would select `PREG_SET_ORDER` or offset capture.

## Writing portable patterns

A pattern that avoids backreferences and lookaround runs on the faster engine with a linear-time guarantee. That is worth doing where the input is untrusted, such as a search box or a route parameter, and not worth contorting a pattern for where the input is your own template source.

```php
// RE2: linear time, no timeout needed.
preg_match_all("/\{([a-z_]+)\}/", $template, $matches);

// Backtracking engine: the closing tag has to equal the opening one.
preg_match_all("/\{(block|inline) (\w+)\}(.*?)\{\/\\1\}/s", $template, $matches);
```

## Implementation

`preg_*` lives in [stdlib/compat/regex.go](../../../stdlib/compat/regex.go), with the rest of the PHP surface whose behaviour is defined by what the interpreter does rather than by what it computes. `compat` is a binding package: the blank import in [stdlib/imports.go](../../../stdlib/imports.go) contributes it through `runner.RegisterBinding`, so a host that wants a different surface builds its runtime without it.

The engine choice lives in `compilePCRE`. The `pattern` type in the same file is the only thing the shims talk to, which is what keeps the two engines indistinguishable from PHP.
