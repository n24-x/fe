package directivefile

import (
	"reflect"
	"testing"
)

func TestParseCommandGoodCases(t *testing.T) {
	// TODO: only the happy path is covered so far. Complete the suite by adding
	// the error branches (missing newline after command, block not starting on a
	// new line, missing '}', unexpected '}', empty block, nesting-depth overflow,
	// and the EOF cases) plus the remaining token kinds and deeper nesting.

	t.Run("1-simple-block", func(t *testing.T) {
		const path = "testsdata/syntax/file/goodcases/1-simple-block.conf"

		tokens, err := Tokenize(readTestdata(t, path), path)
		if err != nil {
			t.Fatalf("Tokenize() error = %v", err)
		}

		p := &parser{tokens: tokens}

		got, err := p.parseCommand(0)
		if err != nil {
			t.Fatalf("parseCommand() error = %v", err)
		}

		want := Command{
			Directive: "proxy",
			Args:      []string{"http"},
			Position:  Position{Filename: path, Line: 1, Column: 1},
			SubCommands: []Command{
				{
					Directive: "listen",
					Args:      []string{"127.0.0.1:8080"},
					Position:  Position{Filename: path, Line: 2, Column: 5},
				},
			},
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("parseCommand():\n got: %#v\nwant: %#v", got, want)
		}

		if p.nextTokenIndex != len(tokens) {
			t.Errorf("nextTokenIndex = %d, want %d (parseCommand did not stop at the trailing newline)",
				p.nextTokenIndex, len(tokens))
		}
	})

	t.Run("2-nested-block", func(t *testing.T) {
		const path = "testsdata/syntax/file/goodcases/2-nested-block.conf"

		tokens, err := Tokenize(readTestdata(t, path), path)
		if err != nil {
			t.Fatalf("Tokenize() error = %v", err)
		}

		p := &parser{tokens: tokens}

		got, err := p.parseCommand(0)
		if err != nil {
			t.Fatalf("parseCommand() error = %v", err)
		}

		want := Command{
			Directive: "server",
			Position:  Position{Filename: path, Line: 1, Column: 1},
			SubCommands: []Command{
				{
					Directive: "listen",
					Args:      []string{"80"},
					Position:  Position{Filename: path, Line: 2, Column: 5},
				},
				{
					Directive: "route",
					Args:      []string{"/api", "backend"},
					Position:  Position{Filename: path, Line: 3, Column: 5},
					SubCommands: []Command{
						{
							Directive: "rewrite",
							Args:      []string{"/v1"},
							Position:  Position{Filename: path, Line: 4, Column: 9},
						},
						{
							Directive: "upstream",
							Args:      []string{"app"},
							Position:  Position{Filename: path, Line: 6, Column: 9},
						},
					},
				},
			},
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("parseCommand():\n got: %#v\nwant: %#v", got, want)
		}

		if p.nextTokenIndex != len(tokens) {
			t.Errorf("nextTokenIndex = %d, want %d (parseCommand did not stop at the trailing newline)",
				p.nextTokenIndex, len(tokens))
		}
	})
}
