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
	// Runs the subcommand
	// The args are the arguments after the subcommand name.
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
	return &Subcommand{
		UsageLine: usageLine,
		Name:      strings.ToLower(data[0]),
		Run:       run,
	}
}
