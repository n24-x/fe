package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/n24-x/fe"
	"github.com/n24-x/fe/feconfig"
	"github.com/n24-x/fe/internal/demo/mcfile"

	_ "github.com/n24-x/fe/internal/demo/modules/standard"
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

	// —— stage [1]: parse (strict) + config semantic validation ——
	mc, err := mcfile.Parse(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parsing machine config: %v\n", err)
		os.Exit(1)
	}
	if err := feconfig.MachineConfigValidate(mc); err != nil {
		fmt.Fprintf(os.Stderr, "machine config validation failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("stage [1] machine config parse + validation: OK")

	// —— stage [2]: runtime-level semantic validation ——
	if err := fe.ValidateRuntimeConfig(mc); err != nil {
		fmt.Fprintf(os.Stderr, "runtime semantic validation failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("stage [2] runtime semantic validation: OK")

	// —— stage [3]+: run through the App (application-shaped) ——
	app, err := fe.New(fe.Options{Name: "demo", SlogHandler: slog.NewTextHandler(os.Stderr, nil)})
	if err != nil {
		fmt.Fprintf(os.Stderr, "new app: %v\n", err)
		os.Exit(1)
	}
	if err := app.Start(mc); err != nil {
		fmt.Fprintf(os.Stderr, "runtime start failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("stage [3]+ runtime start: OK (running; Ctrl-C to stop)")

	// Signal handling is the application's job, not the framework's (issue.md
	// D31): the App only provides the primitive (Start/Stop). The demo shows
	// the wiring: block on SIGINT/SIGTERM, then shut the active Runtime down
	// gracefully.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	if err := app.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "runtime stop failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("stage [3]+ runtime stop: OK")
}
