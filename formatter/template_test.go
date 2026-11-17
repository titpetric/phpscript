package formatter

import (
	"strings"
	"testing"
)

// TestAlignTemplateNestsFunctionAndTags covers both nestings: the class and
// function blocks set the base, a container tag indents its contents, and an
// inline `foreach` nests what follows until its closing line.
func TestAlignTemplateNestsFunctionAndTags(t *testing.T) {
	t.Parallel()
	in := strings.Join([]string{
		"<?php",
		"class Page {",
		"\tfunction rows($items) {",
		"\t\t?>",
		"\t\t<ul>",
		"\t\t<?php foreach ($items as $item) { ?>",
		"\t\t<li><?php echo $item; ?></li>",
		"\t\t<?php } ?>",
		"\t\t</ul>",
		"\t\t<?php",
		"\t}",
		"}",
		"",
	}, "\n")
	want := strings.Join([]string{
		"<?php",
		"class Page {",
		"\tfunction rows($items) {",
		"\t\t?>",
		"\t\t<ul>",
		"\t\t<?php foreach ($items as $item) { ?>",
		"\t\t\t<li><?php echo $item; ?></li>",
		"\t\t<?php } ?>",
		"\t\t</ul>",
		"\t\t<?php",
		"\t}",
		"}",
		"",
	}, "\n")
	if got := alignTemplate(in); got != want {
		t.Fatalf("alignTemplate:\n%s\nwant:\n%s", got, want)
	}
}

// TestAlignTemplateKeepsUnknownMarkupFlat covers a tag the aligner does not
// know: it sets no nesting, and its contents stay at the function depth.
func TestAlignTemplateKeepsUnknownMarkupFlat(t *testing.T) {
	t.Parallel()
	in := strings.Join([]string{
		"<?php",
		"function f() {",
		"\t?>",
		"<widget>",
		"<x>text</x>",
		"</widget>",
		"<?php",
		"}",
		"",
	}, "\n")
	want := strings.Join([]string{
		"<?php",
		"function f() {",
		"\t?>",
		"\t<widget>",
		"\t<x>text</x>",
		"\t</widget>",
		"\t<?php",
		"}",
		"",
	}, "\n")
	if got := alignTemplate(in); got != want {
		t.Fatalf("alignTemplate:\n%s\nwant:\n%s", got, want)
	}
}

// TestAlignTemplateKeepsContents covers the guarantee: only the leading
// whitespace of a line changes.
func TestAlignTemplateKeepsContents(t *testing.T) {
	t.Parallel()
	in := "<?php\nfunction f() {\n  ?>  \n<div>\ntext  with  spaces\n</div>\n<?php\n}\n"
	out := alignTemplate(in)
	inLines := strings.Split(in, "\n")
	outLines := strings.Split(out, "\n")
	if len(inLines) != len(outLines) {
		t.Fatalf("line count changed: %d to %d", len(inLines), len(outLines))
	}
	for i := range inLines {
		if strings.TrimSpace(inLines[i]) != strings.TrimSpace(outLines[i]) {
			t.Errorf("line %d changed: %q to %q", i+1, inLines[i], outLines[i])
		}
	}
}

// TestAlignTemplateIdempotent covers a second pass: each indent comes from
// the source. Aligning an aligned file changes nothing.
func TestAlignTemplateIdempotent(t *testing.T) {
	t.Parallel()
	in := "<?php\nfunction f() {\n\t?>\n\t<ul>\n\t<?php if ($ok) { ?>\n\t<li>x</li>\n\t<?php } ?>\n\t</ul>\n\t<?php\n}\n"
	once := alignTemplate(in)
	if twice := alignTemplate(once); twice != once {
		t.Fatalf("aligning twice changed the file:\n%s\nwant:\n%s", twice, once)
	}
}
