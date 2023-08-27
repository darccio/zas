package zas

import (
	"flag"
	"strings"
)

/*
 * Zas internal subcommand.
 *
 * Inspired by go command.
 */
type Subcommand struct {
	// Runs the subcommand
	// The args are the arguments after the subcommand name.
	Run func()

	// UsageLine is the one-line usage message.
	// The first word in the line is taken to be the subcommand name.
	UsageLine string

	// Name is the name of the subcommand.
	Name string

	// Flag is a set of flags specific to this command.
	Flag flag.FlagSet
}

func NewSubcommand(usageLine string, run func()) *Subcommand {
	data := strings.SplitN(usageLine, " ", 2)
	return &Subcommand{
		UsageLine: usageLine,
		Name:      strings.ToLower(data[0]),
		Run:       run,
	}
}
