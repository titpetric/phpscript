package runner

import (
	"strings"
	"testing"
)

func TestRegisterClassSource(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := New(&out, Options{})
	rt.RegisterClassSource("host:shapes.php", `<?php
interface Shape { public function area(): int; }
class Square implements Shape { private $s; public function __construct($s) { $this->s = $s; } public function area(): int { return $this->s * $this->s; } }
class Circle { public function name(): string { return "circle"; } }
`, "Square", "Circle", "Shape")

	program, err := rt.Load(`<?php
$s = new Square(3);
echo $s->area(), (new Circle)->name(), new Square(2) instanceof Shape ? "Y" : "N";
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatal(err)
	}
	if want := "9circleY"; out.String() != want {
		t.Fatalf("got %q, want %q", out.String(), want)
	}
}
