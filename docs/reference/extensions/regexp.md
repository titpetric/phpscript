# Regular expressions

| PHP feature                           | Status                | Notes                                                                        |
|---------------------------------------|-----------------------|------------------------------------------------------------------------------|
| `preg_match`, `preg_match_all`        | Compatibility         | Both take `$flags` and `$offset`.                                            |
| `PREG_*` match flags                  | Compatibility         | Pattern order, set order, offset capture and unmatched-as-null.              |
| `preg_split`                          | Compatibility         | `$limit` and the three `PREG_SPLIT_*` flags.                                 |
| `preg_replace_callback`               | Partial compatibility | String pattern and subject only; no pattern arrays.                          |
| `preg_replace`                        | Partial compatibility | String pattern, replacement and subject only; no arrays, no `$limit`.        |
| `preg_quote`                          | Compatibility         | Escapes the PCRE metacharacters plus an optional delimiter.                  |
| Backreferences, lookahead, lookbehind | Compatibility         | Compiled by a backtracking engine, with a one-second match timeout.          |
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

One case is decided by the call and not by the pattern. A non-zero `$offset` moves where the match starts without moving where the subject begins, so `^` and `\b` still see the real start of the string. RE2 has no entry point that takes a start position, and slicing `$subject` would move the start, so a call with an offset runs on the backtracking engine whichever one compiled the pattern. A pattern RE2 accepted is compiled a second time, once, on the first call that passes an offset.

RE2 is preferred because it is faster and because a pattern it accepts cannot be made to backtrack catastrophically by hostile input. The fallback engine has no such guarantee, so a match it runs is bounded by a one-second timeout; a pattern that exceeds it reports no match, and the request is not held.

Compilation happens once per pattern per runtime and is cached, so the choice of engine is not paid for on each call.

Which engine ran is not observable from PHP. The same `$matches` shape comes back either way, and a group that did not participate in the match is the empty string in both.

The behaviour this replaced was worse than a missing feature: a pattern RE2 could not compile silently reported "no match", so a template compiler that pairs `{block foo}` with `{/block}` through `\1` produced *wrong output* where an error was due.

## What still differs from PCRE

**Delimiters and modifiers.** A pattern is written PHP-style, `/body/flags`. Any of `( { [ <` may open it, with the matching bracket closing. The modifiers `s`, `m`, `i`, `x` and `U` are translated; anything else (`u`, `D`, `A`, `S`) is accepted and ignored. Both engines match over UTF-8 already, so `u` is mostly redundant, with one exception: PCRE's `u` also makes `\w`, `\d` and `\b` Unicode-aware, and RE2 keeps them ASCII-only. `/(\w)/u` matches `ä` in PHP and does not here.

**Escape sequences.** `\_` and an escaped space are rewritten to the bare character, because both engines reject them as unknown classes where PCRE accepts them.

**Replacements.** `preg_replace` accepts both PHP spellings of a group reference in the replacement, `\1` and `$1`, and normalises them to `${1}` so that a reference followed by a digit, `\1` before a literal `0`, still names group 1.

**Return shapes.** `preg_match` fills `$matches` with the first match's groups. `preg_match_all` fills it in `PREG_PATTERN_ORDER` by default: `$matches[0]` is every whole match, `$matches[1]` every capture of group 1, and so on. `PREG_SET_ORDER` transposes that to one entry per match. Both drop the trailing groups that did not participate, as PHP does, unless `PREG_UNMATCHED_AS_NULL` keeps them as nulls.

**Offsets.** `PREG_OFFSET_CAPTURE` turns every entry into a pair of the matched text and its offset, and `-1` is the offset of a group that did not participate. The offsets are byte offsets, as PHP reports them even under the `u` modifier: a match after a two-byte character starts at 2, not at 1. `$offset` is read the same way, and counts from the end of the subject when it is negative. An `$offset` outside the subject is a failed call, not a failed match: `preg_match` returns `false`.

**Failure.** A pattern that does not compile makes `preg_match` and `preg_match_all` return `false` and `preg_replace_callback` return null, in each case leaving the by-reference argument as the caller left it. PHP also emits a warning, which phpscript has no equivalent of.

## Go's regexp, under its own name

`preg_*` is PHP's surface, and a pattern written for it is a PCRE pattern whichever engine runs it. `Regexp\` is the other direction: Go's `regexp` package reached as PHP classes, with RE2 syntax, RE2 semantics and no PCRE translation layer.

| Class                 | Is                    | Returns          |
|-----------------------|-----------------------|------------------|
| `Regexp\Compile`      | `regexp.Compile`      | `*regexp.Regexp` |
| `Regexp\CompilePOSIX` | `regexp.CompilePOSIX` | `*regexp.Regexp` |

```php
$rx = new Regexp\Compile('(\w+)@(\w+)\.com');
echo $rx->find_string("mail tit@example.com now");
foreach ($rx->find_all_string_submatch($subject, -1) as $match) {
	echo $match[1], "@", $match[2], "\n";
}
```

Both are constructors over the package function of the same name, so an expression that does not parse throws the error `regexp.Compile` returned, catchable by any clause. The value is a `*regexp.Regexp`, so every exported method of that type is callable as PHP spells it: `find_string`, `find_all_string_submatch`, `replace_all_string`, `split`, `num_subexp`, `subexp_names`. `get_class()` answers `Regexp`, which is the Go type behind the value.

Three differences from `preg_*` are the reason to reach for it. Named groups are reported: `$rx->subexp_names()` and `$rx->subexp_index("host")` answer what `$matches["name"]` does not carry. A pattern is compiled once, where the `preg_*` cache is keyed by pattern text per runtime. And `Regexp\CompilePOSIX` has no `preg_*` equivalent at all: it takes the leftmost-longest match, so `a|ab` finds `ab` where the Perl-syntax engines find `a`.

`MustCompile` and `MustCompilePOSIX` are not registered, because they panic where a script needs an error to catch. [Regexp bindings](../../bindings-regexp.md) is the walkthrough, including every fixture.

## Writing portable patterns

A pattern that avoids backreferences and lookaround runs on the faster engine with a linear-time guarantee. That is worth doing where the input is untrusted, such as a search box or a route parameter, and not worth contorting a pattern for where the input is your own template source.

```php
// RE2: linear time, no timeout needed.
preg_match_all("/\{([a-z_]+)\}/", $template, $matches);

// Backtracking engine: the closing tag has to equal the opening one.
preg_match_all("/\{(block|inline) (\w+)\}(.*?)\{\/\\1\}/s", $template, $matches);
```

## Implementation

`preg_*` lives in [stdlib/compat/regex.go](../../../stdlib/compat/regex.go), with the rest of the PHP surface whose behaviour is defined by what the interpreter does and not by what it computes. `compat` is a binding package: the blank import in [stdlib/imports.go](../../../stdlib/imports.go) contributes it through `runner.RegisterBinding`, so a host needing a different surface builds its runtime without it.

The engine choice lives in `compilePCRE`. The `pattern` type in the same file is the only thing the shims talk to, so the two engines are indistinguishable from PHP.

`Regexp\` is a separate package, [stdlib/regexp](../../../stdlib/regexp), because it computes and depends on nothing the interpreter does, and because it shares no code with the PCRE translation: it registers `regexp.Compile` and `regexp.CompilePOSIX` as they are.
