package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

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

func (c *Subcommand) Init() {
	data := strings.SplitN(c.UsageLine, " ", 2)
	c.Name = strings.ToLower(data[0])
}

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
		cmd := exec.Command(fmt.Sprintf("zng%s", args[0]), args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "# %s %s\n", args[0], strings.Join(args[1:], " "))
			panic(err)
		}
	}
}
