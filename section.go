/*
   section - print sections of a text file matching a pattern
   Copyright (C) 2019-2021  Erik Auerswald <auerswal@unix-ag.uni-kl.de>

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU General Public License as published by
   the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU General Public License for more details.

   You should have received a copy of the GNU General Public License
   along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

// section - print indented sections of a text file matching a pattern
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
)

const (
	PROG        = "section"
	VERSION     = "0.0.6"
	ARB_BUF_LIM = 512 * 1024 * 1024 // 500MiB
	DESC        = "prints indented text sections started by matching a pattern."
	COPYRIGHT   = `Copyright (C) 2019-2021 Erik Auerswald <auerswal@unix-ag.uni-kl.de>
License GPLv3+: GNU GPL version 3 or later <http://gnu.org/licenses/gpl.html>.
This is free software: you are free to change and redistribute it.
There is NO WARRANTY, to the extent permitted by law.`
)

// flags
var (
	ignore_case     bool
	omit            bool
	invert_match    bool
	print_help      bool
	print_version   bool
	yaml_ind        bool
	err             error
	in_sect_action  func([]byte) error
	out_sect_action func([]byte) error
)

// regular expressions
var (
	ind_re      = regexp.MustCompile(`^[ \t]*`)
	yaml_ind_re = regexp.MustCompile(`^[ \t]*- `)
	pat_re      *regexp.Regexp
)

// print short usage information
func usage(w io.Writer) {
	fmt.Fprintf(w, "Usage: %s [OPTION...] PATTERN [FILE...]\n", PROG)
}

// XXX: use dedicated error logger instead?
func usage_err(err error) {
	log.SetPrefix(PROG + ": error: ")
	log.Print(err)
	usage(os.Stderr)
	fmt.Fprintf(os.Stderr, "Try '%s -help' for more information\n", PROG)
	os.Exit(2)
}

// print help
func help() {
	version()
	fmt.Println("")
	fmt.Println(PROG, DESC)
	usage(os.Stdout)
	fmt.Println("Options:")
	flag.CommandLine.SetOutput(os.Stdout)
	flag.PrintDefaults()
}

// print version and copyright information
func version() {
	fmt.Println(PROG, "version", VERSION)
	fmt.Println(COPYRIGHT)
}

// print the given line to standard output
func print_line(l []byte) (err error) {
	_, err = os.Stdout.Write(l)
	if err != nil {
		return err
	}
	_, err = os.Stdout.WriteString("\n")
	return err
}

// do not print the given line
func no_output(_ []byte) error {
	return nil
}

// read input text and write matching sections to output
func section(r io.Reader) (matched bool, err error) {
	matched = false  // return if something was matched
	err = nil        // return an error, if one occurs
	in_sect := false // currently inside a section?
	s_ind := 0       // indentation depth of current section
	s := bufio.NewScanner(r)
	var buf []byte
	s.Buffer(buf, ARB_BUF_LIM)
	for s.Scan() {
		l := s.Bytes()
		c_ind := len(ind_re.Find(l))
		c_y_ind := 0
		if yaml_ind {
			c_y_ind = len(yaml_ind_re.Find(l))
		}
		pat_match := pat_re.Match(l)
		if invert_match {
			pat_match = !pat_match
		}
		cont_sect := in_sect && (c_ind > s_ind || c_y_ind > s_ind)
		if pat_match || cont_sect {
			if !in_sect || c_ind < s_ind {
				s_ind = c_ind
			}
			in_sect = true
			matched = true
			err = in_sect_action(l)
			if err != nil {
				log.Print(err)
				return
			}
		} else {
			err = out_sect_action(l)
			if err != nil {
				log.Print(err)
				return
			}
			in_sect = false
			s_ind = 0
		}
	}
	err = s.Err()
	if err != nil {
		log.Print(s.Err())
	}
	return
}

// exit code 2 if an error occurred
// exit code 1 without match nor error
// exit code 0 on match without error
func exit_code(cur int, m bool, err error) (ec int) {
	ec = cur
	if cur == 1 && m {
		ec = 0
	}
	if err != nil {
		ec = 2
	}
	return
}

func main() {
	// XXX: use dedicated error logger instead (go doc log.New)?
	log.SetPrefix(PROG + ": ")
	log.SetFlags(0)

	// initialize actions in- and outside of sections
	in_sect_action = print_line
	out_sect_action = no_output

	// define command line flags
	flag.Usage = func() { usage_err(errors.New("unknown option")) }
	flag.BoolVar(&print_help, "help", false, "display help text and exit")
	flag.BoolVar(&print_help, "h", false, "display help text and exit")
	flag.BoolVar(&print_version, "version", false, "display version and exit")
	flag.BoolVar(&print_version, "V", false, "display version and exit")
	flag.BoolVar(&ignore_case, "ignore-case", false, "ignore case distinctions")
	flag.BoolVar(&ignore_case, "i", false, "ignore case distinctions")
	flag.BoolVar(&yaml_ind, "yaml", false, "allow YAML list indentation")
	flag.BoolVar(&omit, "omit", false, "omit matched sections, print everything else")
	flag.BoolVar(&invert_match, "invert-match", false, "match sections not starting with PATTERN")
	// parse command line flags
	flag.Parse()

	// act on given command line flags
	if print_help {
		help()
		os.Exit(0)
	}
	if print_version {
		version()
		os.Exit(0)
	}
	if omit {
		in_sect_action = no_output
		out_sect_action = print_line
	}
	// required pattern to match on is given as command line argument
	if flag.NArg() < 1 {
		usage_err(errors.New("PATTERN is missing"))
	}
	pat_str := flag.Arg(0)
	// adjust pattern according to command line flags
	if ignore_case {
		pat_str = `(?i)` + pat_str
	}
	pat_re, err = regexp.Compile(pat_str)
	if err != nil {
		usage_err(err)
	}

	ec := 1
	// operate on STDIN if no file name is provided,
	// otherwise operate on the given files
	if flag.NArg() == 1 {
		m, err := section(os.Stdin)
		ec = exit_code(ec, m, err)
	} else {
		for _, arg := range flag.Args()[1:] {
			f, err := os.Open(arg)
			if err != nil {
				log.Print(err)
				continue
			}
			m, err := section(f)
			ec = exit_code(ec, m, err)
			f.Close()
		}
	}
	os.Exit(ec)
}
