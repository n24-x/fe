package directivefile

import "fmt"

func (ctx CommandContext) RegisterTag(tag string, cmd Command) {
	ctx.Context.mu.Lock()
	defer ctx.Context.mu.Unlock()

	_, ok := ctx.Context.instanceRegistry.explicit[tag]
	if ok {
		panic(fmt.Sprintf("%s tag %q is already registered", cmd.Source(), tag))
	}
	id := ctx.Context.newInstanceID()
	ctx.Context.instanceRegistry.explicit[tag] = id
}

// DefaultParseTagFunc parses the optional "tag" sub command.
//
// A command may contain at most one direct "tag" sub command. The "tag"
// command is only recognized at this level and is not processed recursively;
// a "tag" command nested inside another sub command belongs to that sub
// command's module.
//
// SubCommands that are not listed in SubCommandNames are considered unknown.
// If Composable is false, an unknown sub command causes a panic. If
// Composable is true, its module's RegisterTag is called with a child
// CommandContext so that the module can register tags for the unknown
// sub command.
//
// The "tag" command must have exactly one non-empty argument. If no "tag"
// command is present, no tag is registered.
func DefaultParseTagFunc(ctx CommandContext, cmd Command) {
	adapter, err := GetAdapter(ModuleDirective(cmd.Directive))
	if err != nil {
		panic(err)
	}

	handler := adapter.ModuleHandler()
	unmarshaler := handler.Unmarshaler

	var tagCmd *Command

	for i := range cmd.SubCommands {
		sub := &cmd.SubCommands[i]

		if sub.Directive == "tag" {
			if tagCmd != nil {
				panic(fmt.Sprintf(
					"%s: tag command can only appear once",
					sub.Source(),
				))
			}
			tagCmd = sub
			continue
		}

		if _, ok := unmarshaler.SubCommandNames[sub.Directive]; ok {
			continue
		}

		if !unmarshaler.Composable {
			panic(fmt.Sprintf(
				"%s: unknown sub command %q for module %q",
				sub.Source(),
				sub.Directive,
				cmd.Directive,
			))
		}

		subAdapter, err := GetAdapter(ModuleDirective(sub.Directive))
		if err != nil {
			panic(err)
		}

		subHandler := subAdapter.ModuleHandler()
		if subHandler.Unmarshaler.RegisterTag == nil {
			continue
		}

		subCtx := ctx.Child(
			ctx.ParentInstanceID,
			cmd.Directive,
		)

		subHandler.Unmarshaler.RegisterTag(subCtx, *sub)
	}

	if tagCmd != nil {
		if len(tagCmd.Args) != 1 || tagCmd.Args[0] == "" {
			panic(fmt.Sprintf(
				"%s: tag command requires exactly one non-empty argument",
				tagCmd.Source(),
			))
		}

		ctx.RegisterTag(tagCmd.Args[0], *tagCmd)
	}
}
