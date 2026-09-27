#!/usr/bin/env python3
"""Runner for the demo.

The demo is a scratchpad, not part of the fe framework: it rehearses the
Runtime pipeline end to end against a machine config file, as the framework
is built out. It is planned to be removed when v1.0.0 reaches main.

Every command runs from the fe module root, because that is where the module
lives -- the repository root only carries go.work, and running go there
resolves a different package set.

Usage:
    ./make.py                 # run the demo against the bundled config
    ./make.py --config X.json # run it against another config
    ./make.py build           # build the binary instead of running it
    ./make.py test            # run the demo's Go tests
"""

from __future__ import annotations

import argparse
import pathlib
import subprocess
import sys

# make.py sits at fe/internal/demo/make.py, so the module root is three up.
DEMO = pathlib.Path(__file__).resolve().parent
FE_ROOT = DEMO.parents[1]
DEMO_PKG = "./internal/demo/cmd"
DEFAULT_CONFIG = DEMO / "cmd" / "machine-config.json"


def go(args):
    """Run a go command from the module root, returning its exit status."""
    cmd = ["go"] + [str(a) for a in args]
    print("+ " + " ".join(cmd), flush=True)
    try:
        return subprocess.call(cmd, cwd=str(FE_ROOT))
    except FileNotFoundError:
        print("make.py: go is not on PATH", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        # The demo blocks on a signal; Ctrl-C reaching us rather than it is
        # normal, and not a failure of the command.
        return 0


def main(argv):
    parser = argparse.ArgumentParser(
        prog="make.py",
        description="Run, build or test the fe demo.",
    )
    parser.add_argument(
        "command",
        nargs="?",
        default="run",
        choices=("run", "build", "test"),
        help="what to do (default: run)",
    )
    parser.add_argument(
        "--config",
        type=pathlib.Path,
        default=DEFAULT_CONFIG,
        help="machine config to run against (default: the bundled one)",
    )
    parser.add_argument(
        "--out",
        type=pathlib.Path,
        default=DEMO / "fe-demo",
        help="where build writes the binary (default: internal/demo/fe-demo)",
    )
    opts = parser.parse_args(argv)

    if opts.command == "run":
        config = opts.config.resolve()
        if not config.is_file():
            print("make.py: no such config: %s" % config, file=sys.stderr)
            return 1
        return go(["run", DEMO_PKG, config])

    if opts.command == "build":
        return go(["build", "-o", opts.out.resolve(), DEMO_PKG])

    # test: the demo's own package tests (mcfile and the modules).
    return go(["test", "-count=1", "./internal/demo/..."])


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
