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
	// program information
	PROG    = "section"
	VERSION = "0.0.10"
	// technical peculiarities
	ARB_BUF_LIM = 512 * 1024 * 1024 // 512MiB
	// internal regular expressions
	IND_RE      = `^[ \t]*`
	YAML_IND_RE = `^[ \t]*- `
	BLANK_RE    = `^[ \t]*$`
	RE_IGN_CASE = `(?i)`
	// default values
	DEF_SEPARATOR = "--"
	// documentation
	DESC      = "prints indented text sections started by matching a regular expression."
	COPYRIGHT = `Copyright (C) 2019-2021 Erik Auerswald <auerswal@unix-ag.uni-kl.de>
License GPLv3+: GNU GPL version 3 or later <http://gnu.org/licenses/gpl.html>.
This is free software: you are free to change and redistribute it.
There is NO WARRANTY, to the extent permitted by law.`
	OD_HELP             = "display help text and exit"
	OD_IGNORE_BLANK     = "continue sections over blank lines"
	OD_IGNORE_CASE      = "ignore case distinctions"
	OD_INVERT_MATCH     = "match sections not starting with PATTERN"
	OD_OMIT             = "omit matched sections, print everything else"
	OD_QUIET            = "suppress all normal output"
	OD_SEPARATOR        = "print a separator line between sections"
	OD_SEPARATOR_STRING = "specify separator string"
	OD_YAML_IND         = "additionally allow YAML list indentation"
	OD_VERSION          = "display version and exit"
)

type parameters struct {
	// options
	ignore_case      bool
	invert_match     bool
	omit             bool
	quiet            bool
	separator        bool
	separator_string string
	yaml_ind         bool
	// actions performed by the section algorithm
	in_sect_action  func([]byte) error
	out_sect_action func([]byte) error
	// regular expressions matching indentation
	ind_re      *regexp.Regexp
	yaml_ind_re *regexp.Regexp
	// regular expression matching lines to ignore
	ignore_re *regexp.Regexp
	// regular expression matching sections
	pat_re *regexp.Regexp
}

// print short usage information
func usage(w io.Writer) {
	fmt.Fprintf(w, "Usage: %s [OPTION...] PATTERN [FILE...]\n", PROG)
}

// emit an error message
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
func section(p parameters, r io.Reader) (matched bool, err error) {
	matched = false    // return if something was matched
	err = nil          // return an error, if one occurs
	in_sect := false   // currently inside a section?
	cont_sect := false // continue the current section?
	pat_match := false // does current line match pattern?
	s_ind := 0         // indentation depth of current section
	c_ind := 0         // indentation depth of current line
	s_y_ind := 0       // YAML indentation depth of current section
	c_y_ind := 0       // YAML indentation depth of current line
	var buf []byte     // buffer space to hold input data
	var l []byte       // one line of input data
	s := bufio.NewScanner(r)
	s.Buffer(buf, ARB_BUF_LIM)
	for s.Scan() {
		l = s.Bytes()
		if p.ignore_re != nil && p.ignore_re.Match(l) {
			if in_sect {
				err = p.in_sect_action(l)
			} else {
				err = p.out_sect_action(l)
			}
			if err != nil {
				log.Print(err)
				return
			}
			continue
		}
		c_ind = len(p.ind_re.Find(l))
		if p.yaml_ind {
			c_y_ind = len(p.yaml_ind_re.Find(l))
		}
		pat_match = p.pat_re.Match(l)
		if p.invert_match {
			pat_match = !pat_match
		}
		cont_sect = in_sect && (c_ind > s_ind ||
			(s_y_ind >= s_ind && c_y_ind > s_y_ind))
		if p.separator && !p.omit &&
			pat_match && matched && !cont_sect {
			_, err = os.Stdout.WriteString(p.separator_string +
				"\n")
			if err != nil {
				log.Print(err)
				return
			}
		}
		if pat_match || cont_sect {
			if !in_sect || c_ind < s_ind {
				s_ind = c_ind
				s_y_ind = c_y_ind
			}
			in_sect = true
			matched = true
			err = p.in_sect_action(l)
			if err != nil {
				log.Print(err)
				return
			}
		} else {
			err = p.out_sect_action(l)
			if err != nil {
				log.Print(err)
				return
			}
			in_sect = false
			s_ind = 0
			s_y_ind = 0
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
	// for error handling
	var err error

	// parameters for section algorithm
	p := parameters{
		ignore_case:      false,
		invert_match:     false,
		omit:             false,
		separator:        false,
		separator_string: DEF_SEPARATOR,
		quiet:            false,
		yaml_ind:         false,
		in_sect_action:   print_line,
		out_sect_action:  no_output,
		ind_re:           regexp.MustCompile(IND_RE),
		yaml_ind_re:      regexp.MustCompile(YAML_IND_RE),
		ignore_re:        nil,
		pat_re:           nil,
	}

	// error logging
	log.SetPrefix(PROG + ": ")
	log.SetFlags(0)

	// define command line flags
	var print_help, print_version bool
	flag.Usage = func() { usage_err(errors.New("unknown option")) }
	// print program information instead of sections
	flag.BoolVar(&print_help, "help", false, OD_HELP)
	flag.BoolVar(&print_help, "h", false, OD_HELP)
	flag.BoolVar(&print_version, "version", false, OD_VERSION)
	flag.BoolVar(&print_version, "V", false, OD_VERSION)
	// modify section behavior
	flag.BoolVar(&p.ignore_case, "ignore-case", false, OD_IGNORE_CASE)
	flag.BoolVar(&p.ignore_case, "i", false, OD_IGNORE_CASE)
	flag.BoolVar(&p.invert_match, "invert-match", false, OD_INVERT_MATCH)
	flag.BoolVar(&p.omit, "omit", false, OD_OMIT)
	flag.BoolVar(&p.quiet, "quiet", false, OD_QUIET)
	flag.BoolVar(&p.quiet, "q", false, OD_QUIET)
	flag.BoolVar(&p.quiet, "silent", false, OD_QUIET)
	flag.BoolVar(&p.separator, "separator", false, OD_SEPARATOR)
	flag.StringVar(&p.separator_string, "separator-string", DEF_SEPARATOR,
		OD_SEPARATOR_STRING)
	flag.BoolVar(&p.yaml_ind, "yaml", false, OD_YAML_IND)
	ignore_blank := flag.Bool("ignore-blank", false, OD_IGNORE_BLANK)
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
	if p.omit {
		p.in_sect_action = no_output
		p.out_sect_action = print_line
	}
	if p.quiet {
		p.in_sect_action = no_output
		p.out_sect_action = no_output
	}
	if *ignore_blank {
		p.ignore_re = regexp.MustCompile(BLANK_RE)
	}
	// required pattern to match on is given as command line argument
	if flag.NArg() < 1 {
		usage_err(errors.New("PATTERN is missing"))
	}
	pat_str := flag.Arg(0)
	// adjust pattern according to command line flags
	if p.ignore_case {
		pat_str = RE_IGN_CASE + pat_str
	}
	p.pat_re, err = regexp.Compile(pat_str)
	if err != nil {
		usage_err(err)
	}

	ec := 1
	// operate on STDIN if no file name is provided,
	// otherwise operate on the given files
	if flag.NArg() == 1 {
		m, err := section(p, os.Stdin)
		ec = exit_code(ec, m, err)
	} else {
		for _, arg := range flag.Args()[1:] {
			f, err := os.Open(arg)
			if err != nil {
				log.Print(err)
				continue
			}
			m, err := section(p, f)
			ec = exit_code(ec, m, err)
			f.Close()
		}
	}
	os.Exit(ec)
}
