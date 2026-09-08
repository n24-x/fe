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
//	[3]+  run the config through the Manager — the application-shaped path:
//	    Manager.Apply (NewRuntime + Start) → stay up until SIGINT/SIGTERM →
//	    Manager.Stop (reverse stop + graceful exit)
//
// stage [3]+ replaces the earlier manual NewRuntime/Start/Stop rehearsal: a
// config that actually serves (e.g. the TCP-listening endpoint.proxy.server)
// needs to stay up, which is exactly what the Manager is for.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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

	// —— stage [3]+: run through the Manager (application-shaped) ——
	mgr := new(fe.Manager)
	if err := mgr.Apply(&mc); err != nil {
		fmt.Fprintf(os.Stderr, "runtime apply failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("stage [3]+ runtime apply: OK (running; Ctrl-C to stop)")

	// Signal handling is the application's job, not the framework's: the
	// Manager only provides the primitive (Apply/Stop). The demo shows the
	// wiring: block on SIGINT/SIGTERM, then shut the active Runtime down
	// gracefully.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	if err := mgr.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "runtime stop failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("stage [3]+ runtime stop: OK")
}
