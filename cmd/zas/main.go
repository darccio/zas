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
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/darccio/zas"
)

/*
 * Current Zas internal subcommands.
 */
var subcommands = []*zas.Subcommand{
	cmdInit,
	cmdGenerate,
	cmdHelp,
	cmdVersion,
}

var (
	verbose, full *bool
	cmdInit       = zas.NewSubcommand("init - create a new Zas site in the current directory", func() error {
		i := zas.Init{}
		return i.Run()
	})
	cmdGenerate = zas.NewSubcommand("generate - render the site from source into the deploy directory", func() error {
		g := zas.Generator{
			Verbose: *verbose,
			Full:    *full,
		}
		return g.Run()
	})
	// cmdHelp and cmdVersion get their Run funcs wired up in init() below,
	// rather than inline here: both printUsage and printVersion end up
	// referring back to the subcommands slice (to list every command's
	// UsageLine), and subcommands' own initializer lists cmdHelp and
	// cmdVersion. Wiring Run inline would make that an initialization
	// cycle; init() runs after all package-level vars are set up, so
	// assigning it there does not.
	cmdHelp    = zas.NewSubcommand("help - show this help message, or run \"zas <command> -h\" for a command's flags", nil)
	cmdVersion = zas.NewSubcommand("version - print zas version information", nil)
)

func init() {
	verbose = cmdGenerate.Flag.Bool("verbose", false, "Verbose output")
	full = cmdGenerate.Flag.Bool("full", false, "Full generation (non-incremental mode)")

	cmdHelp.Run = func() error {
		printUsage(os.Stdout)
		return nil
	}
	cmdVersion.Run = func() error {
		printVersion(os.Stdout)
		return nil
	}
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		// If no subcommand is provided, we default to "generate".
		args = []string{"generate"}
	}

	if strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "-help", "--help":
			// Top-level help beats the "defaults to generate" rule below:
			// show general help instead of generate's own -h usage.
			args = append([]string{cmdHelp.Name}, args[1:]...)
		case "-version", "--version":
			args = append([]string{cmdVersion.Name}, args[1:]...)
		default:
			// If the first argument is a flag, we default to "generate".
			args = append([]string{"generate"}, args...)
		}
	}

	command := strings.ToLower(args[0])

	for _, cmd := range subcommands {
		if cmd.Name == command && cmd.Run != nil {
			if err := cmd.Flag.Parse(args[1:]); err != nil {
				if errors.Is(err, flag.ErrHelp) {
					return 0
				}
				return 2
			}

			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
				return 1
			}

			return 0
		}
	}

	// No internal subcommand matched; try to exec an external Zas subcommand (plugin).
	return runPlugin(args)
}

func runPlugin(args []string) int {
	cmd := exec.Command(fmt.Sprintf("%s%s", zas.ZAS_PREFIX, args[0]), args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err == nil {
		return 0
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}

	if errors.Is(err, exec.ErrNotFound) {
		fmt.Fprintf(os.Stderr, "unknown command: %q\n", args[0])
	} else {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
	}
	return 127
}

// printUsage writes the top-level help text: what zas is, how to invoke it,
// and the list of internal subcommands with their one-line usage.
func printUsage(w io.Writer) {
	_, _ = fmt.Fprintf(w, "%s is a static site generator.\n\n", zas.ZAS_NAME)
	_, _ = fmt.Fprintf(w, "Usage:\n\n\t%s <command> [arguments]\n\n", zas.ZAS)
	_, _ = fmt.Fprintln(w, "The commands are:")
	_, _ = fmt.Fprintln(w)
	for _, cmd := range subcommands {
		_, _ = fmt.Fprintf(w, "\t%s\n", cmd.UsageLine)
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "Run \"%s <command> -h\" for a command's own flags, or \"%s version\" for version information.\n", zas.ZAS, zas.ZAS)
}

// printVersion writes a best-effort version string for the zas binary,
// derived from Go's module and VCS build-info stamping (runtime/debug,
// available since Go 1.18). For a binary built with "go install
// .../cmd/zas@<version>" this reports that module version; for a plain "go
// build" inside a checkout it typically reports "(devel)" plus the VCS
// revision it was built from.
func printVersion(w io.Writer) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		_, _ = fmt.Fprintf(w, "%s version unknown (no build info available)\n", zas.ZAS)
		return
	}

	version := info.Main.Version
	if version == "" {
		version = "(devel)"
	}

	var revision string
	var dirty bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			dirty = setting.Value == "true"
		}
	}

	_, _ = fmt.Fprintf(w, "%s version %s", zas.ZAS, version)
	if revision != "" {
		if len(revision) > 12 {
			revision = revision[:12]
		}
		_, _ = fmt.Fprintf(w, " (%s", revision)
		if dirty {
			_, _ = fmt.Fprint(w, "-dirty")
		}
		_, _ = fmt.Fprint(w, ")")
	}
	_, _ = fmt.Fprintf(w, " %s\n", info.GoVersion)
}
