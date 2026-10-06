package directivefile

import (
	"fmt"
	"strconv"
	"strings"
)

type File struct {
	Commands []Command
}

type Command struct {
	Directive   string
	Args        []string
	SubCommands []Command
	Origin      Origin
	Position    Position
}

// Origin 描述一个命令是从哪里展开来的（tmpl 调用 / import）。
// 零值表示命令来自本源文件、没有经过任何展开。
type Origin struct {
	TmplPosition   Position
	TemplateName   string
	ImportPosition Position
	ImportChain    []string
}

// HasSubCommands reports whether the command has a nested command block ({ ... }).
func (cmd Command) HasSubCommands() bool {
	return len(cmd.SubCommands) > 0
}

func (cmd Command) Source() string {
	var b strings.Builder

	if cmd.Origin.ImportPosition.IsValid() {
		b.WriteString("imported at ")
		b.WriteString(cmd.Origin.ImportPosition.String())

		if len(cmd.Origin.ImportChain) > 0 {
			b.WriteString(" (")
			b.WriteString(strings.Join(cmd.Origin.ImportChain, " -> "))
			b.WriteString(")")
		}

		if cmd.Origin.TmplPosition.IsValid() {
			b.WriteString("; instantiated from template ")
			b.WriteString(strconv.Quote(cmd.Origin.TemplateName))
			b.WriteString(" at ")
			b.WriteString(cmd.Origin.TmplPosition.String())
		}

		return b.String()
	}

	if cmd.Origin.TmplPosition.IsValid() {
		return fmt.Sprintf(
			"instantiated from template %s at %s",
			strconv.Quote(cmd.Origin.TemplateName),
			cmd.Origin.TmplPosition,
		)
	}

	return cmd.Position.String()
}

// maxCommandDepth is the maximum number of nested command levels.
// Depth starts at 0, so valid depths are 0 through 63.
const maxCommandDepth = 64

// parseCommands parses the whole token stream into top-level commands.
// Every Position is already stamped by the lexer, so no path is needed here.
func (p *parser) parseCommands() ([]Command, error) {
	var cmds []Command

	for {
		tok, index := p.peekToken()
		if index == -1 {
			break
		}

		if tok.Type == NewLine {
			p.nextToken()
			continue
		}

		cmd, err := p.parseCommand(0)
		if err != nil {
			return nil, err
		}

		cmds = append(cmds, cmd)
	}

	return cmds, nil
}

// TODO: introduce a dedicated syntax-error type so every branch reports a
// Position. A few defensive EOF branches still lack a useful location.
func (p *parser) parseCommand(nesting int) (Command, error) {
	var cmd Command

	tok, index := p.nextToken()
	if index == -1 {
		return cmd, fmt.Errorf("syntax: expected directive, got EOF")
	}
	if tok.Type != UnquotedToken {
		return cmd, fmt.Errorf("%s: syntax: expected directive (UnquotedToken), got %s", tok.Position, tok.Type)
	}
	if !validateDirective(tok.Text) {
		return cmd, fmt.Errorf("%s: syntax: invalid directive: %s", tok.Position, tok.Text)
	}
	cmd.Directive = tok.Text
	cmd.Position = tok.Position

	for {
		tok, index = p.peekToken()
		if index == -1 {
			return cmd, fmt.Errorf("%s: syntax: expected newline after command", cmd.Position)
		}

		switch tok.Type {
		case UnquotedToken:
			switch tok.Text {
			case "{":
				leftBracePos := tok.Position

				p.nextToken()

				// Block must start on a new line.
				tok, index = p.nextToken()
				if index == -1 {
					return cmd, fmt.Errorf("%s: syntax: unexpected EOF, expected newline after '{'", leftBracePos)
				}
				if tok.Type != NewLine {
					return cmd, fmt.Errorf("%s: syntax: expected newline after '{' (block must start on a new line)", tok.Position)
				}

				// Additional blank lines are allowed.
				p.skipNewLines()

				subCommands, err := p.parseSubCommands(nesting+1, leftBracePos)
				if err != nil {
					return cmd, err
				}
				cmd.SubCommands = subCommands

				// Next tok must be UnquotedToken and tok.Text = "}"
				tok, index = p.nextToken()
				if index == -1 {
					return cmd, fmt.Errorf("syntax: unexpected EOF, expected '}'")
				}
				if tok.Type != UnquotedToken || tok.Text != "}" {
					return cmd, fmt.Errorf("%s: syntax: expected '}', got %q", tok.Position, tok.Text)
				}
				rightBracePos := tok.Position

				// Command must end with at least one newline.
				tok, index = p.nextToken()
				if index == -1 {
					return cmd, fmt.Errorf("%s: syntax: unexpected EOF, expected newline after '}'", rightBracePos)
				}
				if tok.Type != NewLine {
					return cmd, fmt.Errorf("%s: syntax: expected newline after '}'", tok.Position)
				}

				p.skipNewLines()

				return cmd, nil

			case "}":
				return cmd, fmt.Errorf("%s: syntax: unexpected '}'", tok.Position)

			default:
				p.nextToken()
				arg, err := resolveArg(tok)
				if err != nil {
					return cmd, err
				}
				cmd.Args = append(cmd.Args, arg)
			}

		case DoubleQuotedToken, BacktickQuotedToken:
			p.nextToken()
			arg, err := resolveArg(tok)
			if err != nil {
				return cmd, err
			}
			cmd.Args = append(cmd.Args, arg)

		case NewLine:
			p.nextToken()
			return cmd, nil

		default:
			return cmd, fmt.Errorf("%s: syntax: unexpected token %v", tok.Position, tok.Type)
		}
	}
}

func (p *parser) parseSubCommands(nesting int, leftBracePos Position) ([]Command, error) {
	if nesting >= maxCommandDepth {
		return nil, fmt.Errorf("%s: syntax: command nesting depth exceeds %d",
			leftBracePos,
			maxCommandDepth)
	}

	var cmds []Command

	for {
		tok, index := p.peekToken()
		if index == -1 {
			return nil, fmt.Errorf("%s: syntax: unexpected EOF, expected '}'", leftBracePos)
		}

		switch tok.Type {
		case NewLine:
			p.nextToken()

		case UnquotedToken:
			if tok.Text == "}" {
				if len(cmds) == 0 {
					return nil, fmt.Errorf("%s: syntax: empty block is not allowed", leftBracePos)
				}
				return cmds, nil
			}

			cmd, err := p.parseCommand(nesting)
			if err != nil {
				return nil, err
			}
			cmds = append(cmds, cmd)

		default:
			return nil, fmt.Errorf("%s: syntax: unexpected token %q", tok.Position, tok.Text)
		}
	}
}

func resolveArg(tok Token) (string, error) {
	envName, err := validateEnvReference(tok.Text)
	if err != nil {
		return tok.Text, nil
	}

	value, err := lookupEnv(envName)
	if err != nil {
		return "", fmt.Errorf("%s: %w", tok.Position, err)
	}

	return value, nil
}

// validateDirective reports whether s may name a directive.
func validateDirective(s string) bool {
	if len(s) == 0 {
		return false
	}

	if s[0] == '(' {
		return len(s) >= 3 && s[len(s)-1] == ')'
	}

	for _, segment := range strings.Split(s, ".") {
		if !isDirectiveSegment(segment) {
			return false
		}
	}

	return true
}

// isDirectiveSegment reports whether s is one dot-separated part of a directive
// name: a leading letter, then letters, digits, `_` and `-`. The leading letter
// is what separates a namespace from a name — `dns.2x` is rejected while
// `dns.srv2` is not.
func isDirectiveSegment(s string) bool {
	if len(s) == 0 {
		return false
	}

	if !isASCIILetter(s[0]) {
		return false
	}

	for i := 1; i < len(s); i++ {
		c := s[i]
		if !isASCIILetter(c) && !isASCIIDigit(c) && c != '_' && c != '-' {
			return false
		}
	}

	return true
}
