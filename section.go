/*
   section - print sections of a text file matching a pattern
   Copyright (C) 2019-2022  Erik Auerswald <auerswal@unix-ag.uni-kl.de>

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
	VERSION = "0.3.0"
	// technical peculiarities
	ARB_BUF_LIM = 512 * 1024 * 1024 // 512MiB
	// internal regular expressions
	IND_RE      = `^[ \t]*`
	YAML_IND_RE = `^[ \t]*(- )?`
	BLANK_RE    = `^[ \t]*$`
	RE_IGN_CASE = `(?i)`
	// default values
	DEF_PREFIX_DELIM = ":"
	DEF_SEPARATOR    = "--"
	DEF_STDIN_LABEL  = "(standard input)"
	// documentation
	DESC      = "prints indented text sections started by matching a regular expression."
	COPYRIGHT = `Copyright (C) 2019-2022 Erik Auerswald <auerswal@unix-ag.uni-kl.de>
License GPLv3+: GNU GPL version 3 or later <http://gnu.org/licenses/gpl.html>.
This is free software: you are free to change and redistribute it.
There is NO WARRANTY, to the extent permitted by law.`
	OD_HELP             = "display help text and exit"
	OD_IGNORE_BLANK     = "continue sections over blank lines"
	OD_IGNORE_CASE      = "ignore case distinctions"
	OD_INVERT_MATCH     = "match sections not starting with PATTERN"
	OD_LINE_NUMBER      = "prefix output lines with line number"
	OD_OMIT             = "omit matched sections, print everything else"
	OD_OMIT_IGNORED     = "omit lines ignored as section breaks"
	OD_PREFIX_DELIM     = "string to delimit a prefix"
	OD_QUIET            = "suppress all normal output"
	OD_SEPARATOR        = "print a separator line between sections"
	OD_SEPARATOR_STRING = "specify separator string"
	OD_STDIN_LABEL      = "label in place of file name for standard input"
	OD_WITH_FILENAME    = "prefix output lines with file name"
	OD_YAML_IND         = "additionally allow YAML list indentation"
	OD_VERSION          = "display version and exit"
)

// parameterize section algorithm
type section_params struct {
	// options
	ignore_case  bool
	invert_match bool
	stdin_label  string
	// actions performed by the section algorithm
	action *line_printer
	ignore *line_printer
	// regular expressions matching indentation
	ind_re *regexp.Regexp
	// regular expression matching lines to ignore
	ignore_re *regexp.Regexp
	// regular expression matching sections
	pat_re *regexp.Regexp
}

// line printer object
type line_printer struct {
	// state
	has_printed bool
	quiet       bool
	// values
	filename         string
	prefix_delim     string
	separator_string string
	// features
	line_number   bool
	omit          bool
	separator     bool
	with_filename bool
}

// method to possibly print a line, depending on state and parameters
func (p *line_printer) print_line(l *[]byte, nr uint64, tr bool, is bool) (err error) {
	if p.quiet || (p.omit && is) || (!p.omit && !is) {
		return nil
	}
	if p.separator && p.has_printed && tr {
		_, err = os.Stdout.WriteString(p.separator_string + "\n")
		if err != nil {
			return
		}
	}
	if p.with_filename {
		_, err = os.Stdout.WriteString(p.filename + p.prefix_delim)
		if err != nil {
			return
		}
	}
	if p.line_number {
		_, err = fmt.Printf("%d%s", nr, p.prefix_delim)
		if err != nil {
			return
		}
	}
	_, err = os.Stdout.Write(*l)
	if err != nil {
		return
	}
	p.has_printed = true
	_, err = os.Stdout.WriteString("\n")
	return
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

// read input text and write matching sections to output
func section(p section_params, r io.Reader) (matched bool, err error) {
	matched = false    // return if something was matched
	err = nil          // return an error, if one occurs
	in_sect := false   // currently inside a section?
	cont_sect := false // continue the current section?
	pat_match := false // does current line match pattern?
	s_ind := 0         // indentation depth of current section
	c_ind := 0         // indentation depth of current line
	var buf []byte     // buffer space to hold input data
	var l []byte       // one line of input data
	var l_nr uint64    // current line number
	var tr bool        // transition into or out of section?

	// process input line by line
	s := bufio.NewScanner(r)
	s.Buffer(buf, ARB_BUF_LIM)
	for s.Scan() {
		l_nr++
		l = s.Bytes()
		// ignored lines do not cause a section transition
		if p.ignore_re != nil && p.ignore_re.Match(l) {
			err = p.ignore.print_line(&l, l_nr, false, in_sect)
			if err != nil {
				log.Print(err)
				return
			}
			continue
		}
		// determine indentation depth of current line
		c_ind = len(p.ind_re.Find(l))
		// check if current line matches pattern
		pat_match = p.pat_re.Match(l)
		if p.invert_match {
			pat_match = !pat_match
		}
		// is the current line a continuation of a section?
		cont_sect = in_sect && (c_ind > s_ind)
		// is the current line a transition out of / into a section,
		// or from one section into another?
		tr = (in_sect && !cont_sect) || (!in_sect && pat_match)
		// update section state variables
		if pat_match || cont_sect {
			if !in_sect || c_ind < s_ind {
				s_ind = c_ind
			}
			in_sect = true
			matched = true
		} else {
			in_sect = false
			s_ind = 0
		}
		// invoke line action according to current situation
		err = p.action.print_line(&l, l_nr, tr, in_sect)
		if err != nil {
			log.Print(err)
			return
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
	sp := section_params{
		stdin_label: DEF_STDIN_LABEL,
		ind_re:      regexp.MustCompile(IND_RE),
	}
	// default line printer
	lp := line_printer{
		separator_string: DEF_SEPARATOR,
		filename:         "",
	}
	// line printer as normal and ignore action by default
	sp.action = &lp
	sp.ignore = &lp

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
	flag.BoolVar(&sp.ignore_case, "ignore-case", false, OD_IGNORE_CASE)
	flag.BoolVar(&sp.ignore_case, "i", false, OD_IGNORE_CASE)
	flag.BoolVar(&sp.invert_match, "invert-match", false, OD_INVERT_MATCH)
	flag.StringVar(&sp.stdin_label, "label", DEF_STDIN_LABEL,
		OD_STDIN_LABEL)
	flag.BoolVar(&lp.line_number, "line-number", false, OD_LINE_NUMBER)
	flag.BoolVar(&lp.line_number, "n", false, OD_LINE_NUMBER)
	flag.BoolVar(&lp.omit, "omit", false, OD_OMIT)
	flag.StringVar(&lp.prefix_delim, "prefix-delimiter", DEF_PREFIX_DELIM,
		OD_PREFIX_DELIM)
	flag.BoolVar(&lp.quiet, "quiet", false, OD_QUIET)
	flag.BoolVar(&lp.quiet, "q", false, OD_QUIET)
	flag.BoolVar(&lp.quiet, "silent", false, OD_QUIET)
	flag.BoolVar(&lp.separator, "separator", false, OD_SEPARATOR)
	flag.StringVar(&lp.separator_string, "separator-string", DEF_SEPARATOR,
		OD_SEPARATOR_STRING)
	flag.BoolVar(&lp.with_filename, "with-filename", false,
		OD_WITH_FILENAME)
	yaml_ind := flag.Bool("yaml", false, OD_YAML_IND)
	ignore_blank := flag.Bool("ignore-blank", false, OD_IGNORE_BLANK)
	omit_ignored := flag.Bool("omit-ignored", false, OD_OMIT_IGNORED)
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
	if *ignore_blank {
		sp.ignore_re = regexp.MustCompile(BLANK_RE)
	}
	if *omit_ignored {
		no_output := line_printer{
			quiet: true,
		}
		sp.ignore = &no_output
	}
	if *yaml_ind {
		sp.ind_re = regexp.MustCompile(YAML_IND_RE)
	}
	// required pattern to match on is given as command line argument
	if flag.NArg() < 1 {
		usage_err(errors.New("PATTERN is missing"))
	}
	pat_str := flag.Arg(0)
	// adjust pattern according to command line flags
	if sp.ignore_case {
		pat_str = RE_IGN_CASE + pat_str
	}
	sp.pat_re, err = regexp.Compile(pat_str)
	if err != nil {
		usage_err(err)
	}

	ec := 1
	// operate on STDIN if no file name is provided,
	// otherwise operate on the given files
	if flag.NArg() == 1 {
		lp.filename = sp.stdin_label
		m, err := section(sp, os.Stdin)
		ec = exit_code(ec, m, err)
	} else {
		for _, arg := range flag.Args()[1:] {
			f, err := os.Open(arg)
			if err != nil {
				log.Print(err)
				continue
			}
			lp.filename = arg
			m, err := section(sp, f)
			ec = exit_code(ec, m, err)
			f.Close()
		}
	}
	os.Exit(ec)
}
