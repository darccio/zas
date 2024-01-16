/*
 * Copyright (c) 2013 Dario Castañé.
 * This file is part of Zas.
 *
 * Zas is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * Zas is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with Zas.  If not, see <http://www.gnu.org/licenses/>.
 */
package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/darccio/zas"
)

/*
 * Current Zas internal subcommands.
 */
var subcommands = []*zas.Subcommand{
	cmdInit,
	cmdGenerate,
}

var (
	cmdInit = zas.NewSubcommand("init", func() {
		i := zas.Init{}
		i.Run()
	})
	cmdGenerate = zas.NewSubcommand("generate", func() {
		g := zas.Generator{
			Verbose: *verbose,
			Full:    *full,
		}
		g.Run()
	})
	verbose, full *bool
)

func init() {
	verbose = cmdGenerate.Flag.Bool("verbose", false, "Verbose output")
	full = cmdGenerate.Flag.Bool("full", false, "Full generation (non-incremental mode)")
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	args := os.Args[1:]
	if len(args) == 0 {
		// If no subcommand is provided, we default to "generate".
		args = []string{"generate"}
	}

	if strings.HasPrefix(args[0], "-") {
		// If the first argument is a flag, we default to "generate".
		args = append([]string{"generate"}, args...)
	}

	var (
		command = strings.ToLower(args[0])
		found = false
	)

	for _, cmd := range subcommands {
		if cmd.Name == command && cmd.Run != nil {
			found = true

			cmd.Flag.Parse(args[1:])
			cmd.Run()

			break
		}
	}

	if found {
		// If an internal subcommand is found, we exit.
		os.Exit(0)
	}

	// If not internal subcommand is found, we try to exec an external Zas subcommand (plugin).
	cmd := exec.Command(fmt.Sprintf("%s%s", zas.ZAS_PREFIX, args[0]), args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "# %s %s\n", args[0], strings.Join(args[1:], " "))

		panic(err)
	}
}
