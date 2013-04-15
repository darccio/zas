/*
 * Copyright (c) 2013 Dario Castañé.
 * This file is part of Zingy.
 *
 * Zingy is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * Zingy is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with Zingy.  If not, see <http://www.gnu.org/licenses/>.
 */
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

/*
 * Zingy internal subcommand.
 * 
 * Inspired by go command.
 */
type Subcommand struct {
	// Runs the subcommand
	// The args are the arguments after the subcommand name.
	Run func()

	// UsageLine is the one-line usage message.
	// The first word in the line is taken to be the subcommand name
	// with Init() function.
	UsageLine string

	// Name is the name of the subcommand.
	Name string

	// Flag is a set of flags specific to this command.
	Flag flag.FlagSet
}

/*
 * Subcommand init function.
 */
func (c *Subcommand) Init() {
	data := strings.SplitN(c.UsageLine, " ", 2)
	c.Name = strings.ToLower(data[0])
}

/*
 * Current Zingy internal subcommands.
 */
var subcommands = []*Subcommand{
	cmdInit,
	cmdGenerate,
}

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) > 0 {
		args[0] = strings.ToLower(args[0])
	} else {
		// If no subcommand is provided, we default to "generate".
		args = []string{"generate"}
	}

	runtime.GOMAXPROCS(runtime.NumCPU())
	found := false
	for _, cmd := range subcommands {
		if cmd.Name == args[0] && cmd.Run != nil {
			found = true
			cmd.Flag.Parse(args[1:])
			cmd.Run()
			break
		}
	}
	if !found {
		// If not internal subcommand is found, we try to exec an external Zingy subcommand (plugin).
		cmd := exec.Command(fmt.Sprintf("%s%s", ZNG_PREFIX, args[0]), args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "# %s %s\n", args[0], strings.Join(args[1:], " "))
			panic(err)
		}
	}
}
