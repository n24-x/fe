// Command fe-demo rehearses the Runtime pipeline end to end against a
// machine config file, one stage at a time, as the framework is built out.
//
// It lives under internal/testing: it is a development scratchpad, not part
// of the fe framework. It registers test modules by importing them; the test
// modules live in ../modules.
//
// Implemented stages:
//
//	[1] read machine config + syntax validation (feconfig.MachineConfigValidate)
//	[2] runtime-level semantic validation (fe.ValidateRuntimeConfig)
//	[3] build Runtime: resolve creation order via dag-go, then provision every
//	    instance in that order (fe.NewRuntime). Instances are created but not
//	    started yet.
//	[4] start instances → Running (fe.Runtime.Start), then shut them down
//	    again (fe.Runtime.Stop, reverse start order)
//
// Not started:
//
//	[5] reload (Manager.Apply)
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/n24-x/fe"
	"github.com/n24-x/fe/feconfig"

	// Register test modules via import side effect (their init() calls
	// fe.RegisterModule). Without importing a module's package, its module
	// type is not registered and runtime semantic validation rejects it.
	_ "github.com/n24-x/fe/internal/testing/modules/dns"
	_ "github.com/n24-x/fe/internal/testing/modules/dnsforwarder"
	_ "github.com/n24-x/fe/internal/testing/modules/endpointproxy"
	_ "github.com/n24-x/fe/internal/testing/modules/logger"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: fe-demo <machine-config.json>")
		os.Exit(2)
	}

	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read config: %v\n", err)
		os.Exit(1)
	}

	// —— stage [1]: syntax validation ——
	if err := feconfig.MachineConfigValidate(raw); err != nil {
		fmt.Fprintf(os.Stderr, "syntax validation failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("stage [1] syntax validation: OK")

	// —— stage [2]: runtime-level semantic validation ——
	var mc feconfig.MachineConfig
	if err := json.Unmarshal(raw, &mc); err != nil {
		fmt.Fprintf(os.Stderr, "decode machine config: %v\n", err)
		os.Exit(1)
	}
	if err := fe.ValidateRuntimeConfig(&mc); err != nil {
		fmt.Fprintf(os.Stderr, "runtime semantic validation failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("stage [2] runtime semantic validation: OK")

	// —— stage [3]: build Runtime (instantiate every instance, decode config) ——
	rt, err := fe.NewRuntime(&mc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "runtime instantiation failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("stage [3] runtime instantiation: OK")

	// —— stage [4]: start instances → Running, then shut down ——
	if err := rt.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "runtime start failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("stage [4] runtime start: OK (running)")

	if err := rt.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "runtime stop failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("stage [4] runtime stop: OK")

	// TODO stage [5]: reload (Manager.Apply).
}
