package directivefile

import (
	"context"

	"github.com/n24-x/fe/feconfig"
)

func Adapt(path string, opts feconfig.Options) (feconfig.MachineConfig, error) {
	ctx := NewContext(context.Background())
	mc := feconfig.MachineConfig{
		Options: opts,
	}

	dfCmds, err := parse(path)
	if err != nil {
		return feconfig.MachineConfig{}, err
	}

	err = ctx.registerTags(dfCmds.Commands)
	if err != nil {
		return feconfig.MachineConfig{}, err
	}

	cmds := ctx.registerDirectInstantiateModule()
	cmds = append(cmds, dfCmds.Commands...)

	err = ctx.run(&mc, cmds)
	if err != nil {
		return feconfig.MachineConfig{}, err
	}

	return mc, nil
}
