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

package zas

import (
	"flag"
	"strings"
)

// Subcommand is a Zas internal subcommand, inspired by the go command.
type Subcommand struct {
	// Run runs the subcommand. It takes no arguments - flags are package
	// globals wired up in init() - and returns an error instead of
	// panicking or exiting directly.
	Run func() error

	// UsageLine is the one-line usage message.
	// The first word in the line is taken to be the subcommand name.
	UsageLine string

	// Name is the name of the subcommand.
	Name string

	// Flag is a set of flags specific to this command.
	Flag flag.FlagSet
}

// NewSubcommand builds a Subcommand from a usage line and its run function.
func NewSubcommand(usageLine string, run func() error) *Subcommand {
	data := strings.SplitN(usageLine, " ", 2)
	name := strings.ToLower(data[0])

	// Named (rather than the zero value) so that Go's own flag-parsing
	// error/usage output - e.g. from an unrecognized flag, or -h/-help on a
	// subcommand with no Usage func set - reports "Usage of <name>:"
	// instead of "Usage of :".
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	// flag.NewFlagSet points fs.Usage at fs.defaultUsage, a method value
	// bound to the *flag.FlagSet it returns. Subcommand.Flag stores a
	// flag.FlagSet by value, so fs gets copied below; a bound Usage would
	// keep referring to this now-orphaned fs instead of the copy that
	// callers actually register flags on and parse with, so it would
	// print an empty flag list. Clearing it here restores the ordinary
	// (nil-Usage) behavior, where usage() calls defaultUsage() on
	// whichever FlagSet it's actually invoked on.
	fs.Usage = nil

	return &Subcommand{
		UsageLine: usageLine,
		Name:      name,
		Run:       run,
		Flag:      *fs,
	}
}
