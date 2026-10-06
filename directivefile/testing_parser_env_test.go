package directivefile

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

const envDir = "testsdata/syntax/env/"

// unsetEnv removes name for the duration of the test and restores it after.
// t.Setenv cannot express "unset", and the not-set case needs exactly that.
func unsetEnv(t *testing.T, name string) {
	t.Helper()

	old, had := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("unsetenv(%s): %v", name, err)
	}
	t.Cleanup(func() {
		if had {
			os.Setenv(name, old)
			return
		}
		os.Unsetenv(name)
	})
}

// Every test here mutates the process environment, so none may call
// t.Parallel — t.Setenv refuses to run in a parallel test anyway.

// TestParseEnvGoodCases covers both outcomes of the shape test: a real
// placeholder is substituted, and anything that is not exactly {$NAME} is
// passed through as a literal instead of being rejected.
func TestParseEnvGoodCases(t *testing.T) {
	const value = "223.5.5.5"

	// X is SET on purpose: it makes 2-literal-embedded.conf meaningful — there
	// the name exists, yet `pre{$X}post` must still stay verbatim (no
	// concatenation).
	t.Setenv("DF_TEST_ENV_VALUE", value)
	t.Setenv("X", "ex")

	cases := []struct {
		file string
		args []string
		line int
	}{
		// The positive case: substitution really happened.
		{"0-value-arg.conf", []string{value}, 10},
		// Literals: the shape test failed, so the text is untouched.
		{"2-literal-colon.conf", []string{"{$A:B}"}, 5},
		{"2-literal-digit-name.conf", []string{"{$1}"}, 7},
		{"2-literal-embedded.conf", []string{"pre{$X}post"}, 5},
		{"2-literal-empty-name.conf", []string{"{$}"}, 5},
	}

	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := envDir + tc.file

			file, err := parse(path)
			if err != nil {
				t.Fatalf("parse() error = %v", err)
			}

			want := File{Commands: []Command{{
				Directive: "cmd",
				Args:      tc.args,
				Position:  Position{Filename: path, Line: tc.line, Column: 1},
			}}}
			if !reflect.DeepEqual(file, want) {
				t.Fatalf("parse():\n got: %#v\nwant: %#v", file, want)
			}
		})
	}
}

// TestParseEnvBadCases covers the three ways an env placeholder can be
// rejected, and where the error points.
func TestParseEnvBadCases(t *testing.T) {
	t.Run("1-directive-position", func(t *testing.T) {
		const path = envDir + "1-directive-position.conf"

		// The variable IS set: the rejection is about the POSITION, so this
		// case must fail for the right reason. Were it unset we could not tell
		// a position rule from a missing value.
		t.Setenv("PROXY", "proxy")

		_, err := parse(path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		// Substitution only happens while reading Args, so the directive token
		// reaches validateDirective untouched and fails its shape rule.
		wantErr := path + ":10:1: syntax: invalid directive: {$PROXY}"
		if !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("expected error containing %q, got %q", wantErr, err)
		}
	})

	t.Run("3-not-set", func(t *testing.T) {
		const path = envDir + "3-not-set.conf"

		unsetEnv(t, "DF_TEST_ENV_NOT_SET")

		_, err := parse(path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		// Reported at the placeholder itself (line 8, column 5), not at the
		// start of the command.
		wantErr := path + `:8:5: environment variable "DF_TEST_ENV_NOT_SET" is not set`
		if !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("expected error containing %q, got %q", wantErr, err)
		}
	})

	t.Run("4-empty", func(t *testing.T) {
		const path = envDir + "4-empty.conf"

		// Set but empty. The wording differs from the not-set case, which is
		// the only observable difference between the two — so assert it.
		t.Setenv("DF_TEST_ENV_EMPTY", "")

		_, err := parse(path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		wantErr := path + `:8:5: environment variable "DF_TEST_ENV_EMPTY" is empty`
		if !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("expected error containing %q, got %q", wantErr, err)
		}
	})
}
