package directivefile

import (
	"fmt"
	"os"
	"strings"
)

func parse(path string) (File, error) {
	return parseFile(path, Origin{})
}

func parseFile(path string, origin Origin) (File, error) {
	chain := append(append([]string(nil), origin.ImportChain...), path)
	if len(chain) > maxImportDepth {
		return File{}, fmt.Errorf("%s: syntax: import nesting exceeds %d levels: %s",
			path, maxImportDepth, strings.Join(chain, " -> "))
	}
	origin.ImportChain = chain

	input, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	tokens, err := Tokenize(input, path)
	if err != nil {
		return File{}, err
	}

	p := &parser{tokens: tokens}

	cmds, err := p.parseCommands()
	if err != nil {
		return File{}, err
	}

	// expand template
	templates, cmds, err := extractTemplates(cmds)
	if err != nil {
		return File{}, err
	}

	if len(cmds) == 0 && len(templates) == 0 {
		return File{}, fmt.Errorf("syntax: file %s must contain at least one command", path)
	}

	cmds, err = expandTemplates(cmds, templates)
	if err != nil {
		return File{}, err
	}

	// expand import
	cmds, err = expandImports(cmds, path, origin)
	if err != nil {
		return File{}, err
	}

	// validate file
	err = validateFile(cmds)
	if err != nil {
		return File{}, err
	}

	return File{Commands: cmds}, nil
}

func validateFile(cmds []Command) error {
	return validateCommands(cmds, 1)
}

func validateCommands(cmds []Command, depth int) error {
	if depth > maxCommandDepth {
		return fmt.Errorf(
			"syntax: command nesting depth exceeds maximum of %d",
			maxCommandDepth,
		)
	}
	for _, cmd := range cmds {
		switch {
		case cmd.Directive == "import":
			return fmt.Errorf(
				"%s: syntax: import command must be expanded",
				cmd.Source(),
			)
		case cmd.Directive == "tmpl":
			return fmt.Errorf(
				"%s: syntax: tmpl command must be expanded",
				cmd.Source(),
			)
		case !isNormalDirective(cmd.Directive):
			return fmt.Errorf(
				"%s: syntax: template directive %s must be expanded",
				cmd.Source(),
				cmd.Directive,
			)
		}
		if len(cmd.SubCommands) > 0 {
			if err := validateCommands(cmd.SubCommands, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

type parser struct {
	tokens         []Token
	nextTokenIndex int // 当前的 token 下标
}

// nextToken returns the next Token and its index.
// It returns -1 as the index when all tokens have been consumed.
func (p *parser) nextToken() (Token, int) {
	if p.nextTokenIndex >= len(p.tokens) {
		return Token{}, -1
	}
	index := p.nextTokenIndex
	p.nextTokenIndex++
	return p.tokens[index], index
}

// peekToken returns the next Token without consuming it.
func (p *parser) peekToken() (Token, int) {
	if p.nextTokenIndex >= len(p.tokens) {
		return Token{}, -1
	}
	return p.tokens[p.nextTokenIndex], p.nextTokenIndex
}

func (p *parser) skipNewLines() {
	for {
		tok, index := p.peekToken()
		if index == -1 || tok.Type != NewLine {
			return
		}
		p.nextToken()
	}
}
