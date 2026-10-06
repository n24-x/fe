package directivefile

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestExpandImportsGoodCase(t *testing.T) {
	t.Run("imported file holds a block", func(t *testing.T) {
		const path = "testsdata/syntax/import/goodcases/1-import-block.conf"
		const target = "testsdata/syntax/import/fixtures/block.conf"

		file, err := parse(path)
		if err != nil {
			t.Fatalf("parse() error = %v", err)
		}

		// Every command that came from the imported file carries the same
		// Origin: the import site in THIS file, plus the chain that reached it.
		// The nested commands matter most — propagating the Origin into
		// SubCommands was a real regression, and a flat-only fixture would not
		// have caught it.
		imported := Origin{
			ImportPosition: Position{Filename: path, Line: 8, Column: 1},
			ImportChain:    []string{path, target},
		}

		want := File{Commands: []Command{
			{
				Directive: "aaa",
				Position:  Position{Filename: path, Line: 7, Column: 1},
			},
			{
				Directive: "top",
				Position:  Position{Filename: target, Line: 1, Column: 1},
				Origin:    imported,
				SubCommands: []Command{
					{
						Directive: "inner1",
						Position:  Position{Filename: target, Line: 2, Column: 5},
						Origin:    imported,
					},
					{
						Directive: "inner2",
						Position:  Position{Filename: target, Line: 3, Column: 5},
						Origin:    imported,
						SubCommands: []Command{
							{
								Directive: "deep",
								Position:  Position{Filename: target, Line: 4, Column: 9},
								Origin:    imported,
							},
						},
					},
				},
			},
			{
				Directive: "bbb",
				Position:  Position{Filename: path, Line: 9, Column: 1},
			},
		}}

		if !reflect.DeepEqual(file, want) {
			t.Fatalf("parse():\n got: %#v\nwant: %#v", file, want)
		}
	})
}

func TestExpandImportBadCase(t *testing.T) {
	t.Run("empty imported file", func(t *testing.T) {
		const path = "testsdata/syntax/import/badcases/3-import-empty-file.conf"
		const target = "testsdata/syntax/import/fixtures/empty.conf"

		_, err := parse(path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		// The rejection happens in parseFile, before the import site knows
		// anything, so the message names the IMPORTED file. Note it carries no
		// line:column — a defs-only file is caught later instead, by
		// expandImports, and that one does report at the import line.
		wantErr := "syntax: file " + target + " must contain at least one command"
		if !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("expected error containing %q, got %q", wantErr, err)
		}
	})

	t.Run("defs-only imported file", func(t *testing.T) {
		const path = "testsdata/syntax/import/badcases/4-import-defs-only-file.conf"
		const target = "testsdata/syntax/import/fixtures/defs-only.conf"

		_, err := parse(path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		// This one is caught by expandImports, not parseFile: the definition
		// makes the file non-empty, so the import site is the first place that
		// can report it — and unlike the empty-file case, that gives us a
		// position. Asserting BOTH halves pins the difference.
		wantErr := fmt.Sprintf("%s:13:1: syntax: imported file %q must contain at least one command", path, target)
		if !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("expected error containing %q, got %q", wantErr, err)
		}
	})
}

func TestExpandImportsBadCaseCycleImport(t *testing.T) {
	t.Run("import self", func(t *testing.T) {
		const path = "testsdata/syntax/import/badcases/1-self-import.conf"

		_, err := parse(path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		const wantErr = "import cycle"
		if !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("expected error containing %q, got %q", wantErr, err)
		}

		// The cycle closes on the first hop, so the chain lists the same file
		// twice. The position is the `import` line in the file itself, which
		// also proves the check ran BEFORE the file was read a second time.
		wantChain := path + " -> " + path
		if !strings.Contains(err.Error(), wantChain) {
			t.Fatalf("expected chain %q, got %q", wantChain, err)
		}
		if wantPos := path + ":6:1"; !strings.Contains(err.Error(), wantPos) {
			t.Fatalf("expected error at %s, got %q", wantPos, err)
		}
	})

	t.Run("two files import each other", func(t *testing.T) {
		const path = "testsdata/syntax/import/badcases/2-two-file-cycle.conf"
		const a = "testsdata/syntax/import/fixtures/cycle-a.conf"
		const b = "testsdata/syntax/import/fixtures/cycle-b.conf"

		_, err := parse(path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		const wantErr = "import cycle"
		if !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("expected error containing %q, got %q", wantErr, err)
		}

		// The cycle closes inside b, not at the entry point, so the error must
		// name b's own import line. Reporting it at `path` instead would still
		// be an error, but a much less useful one.
		if wantPos := b + ":4:1"; !strings.Contains(err.Error(), wantPos) {
			t.Fatalf("expected error at %s, got %q", wantPos, err)
		}

		// The chain must be the whole walk that reached the closure, in order,
		// with the closing file listed twice.
		wantChain := strings.Join([]string{path, a, b, a}, " -> ")
		if !strings.Contains(err.Error(), wantChain) {
			t.Fatalf("expected chain %q, got %q", wantChain, err)
		}
	})
}
