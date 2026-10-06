package directivefile

import (
	"fmt"
	"path/filepath"
	"strings"
)

const maxImportDepth = 32

func expandImports(cmds []Command, path string, origin Origin) ([]Command, error) {
	var result []Command

	for _, cmd := range cmds {
		if cmd.Directive != "import" {
			subCommands, err := expandImports(cmd.SubCommands, path, origin)
			if err != nil {
				return nil, err
			}
			cmd.SubCommands = subCommands

			result = append(result, cmd)
			continue
		}

		if len(cmd.Args) == 0 {
			return nil, fmt.Errorf(
				"%s: syntax: import requires at least one argument",
				cmd.Position,
			)
		}

		if cmd.HasSubCommands() {
			return nil, fmt.Errorf(
				"%s: syntax: import command cannot have a block",
				cmd.Position,
			)
		}

		var imported []Command

		for _, arg := range cmd.Args {
			candidate := filepath.Join(filepath.Dir(path), arg)

			if onImportChain(origin.ImportChain, candidate) {
				chain := append(
					append([]string(nil), origin.ImportChain...),
					candidate,
				)

				return nil, fmt.Errorf(
					"%s: syntax: import cycle: %s",
					cmd.Position,
					formatImportChain(chain),
				)
			}

			childOrigin := Origin{
				ImportChain: append(
					[]string(nil),
					origin.ImportChain...,
				),
			}

			file, err := parseFile(candidate, childOrigin)
			if err != nil {
				return nil, err
			}

			if len(file.Commands) == 0 {
				return nil, fmt.Errorf(
					"%s: syntax: imported file %q must contain at least one command",
					cmd.Position,
					candidate,
				)
			}

			chain := append(
				append([]string(nil), origin.ImportChain...),
				candidate,
			)

			origin := Origin{
				ImportPosition: cmd.Position,
				ImportChain:    chain,
			}
			setImportOrigin(file.Commands, origin)

			imported = append(imported, file.Commands...)
		}

		result = append(result, imported...)
	}

	return result, nil
}

func onImportChain(chain []string, path string) bool {
	for _, item := range chain {
		if item == path {
			return true
		}
	}

	return false
}

func formatImportChain(chain []string) string {
	return strings.Join(chain, " -> ")
}

func setImportOrigin(cmds []Command, origin Origin) {
	for i := range cmds {
		if !cmds[i].Origin.ImportPosition.IsValid() {
			cmds[i].Origin.ImportPosition = origin.ImportPosition
			cmds[i].Origin.ImportChain = origin.ImportChain
		}
		setImportOrigin(cmds[i].SubCommands, origin)
	}
}
