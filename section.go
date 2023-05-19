/*
   section - print sections of a text file matching a pattern
   Copyright (C) 2019-2023  Erik Auerswald <auerswal@unix-ag.uni-kl.de>

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
	VERSION = "0.7.0+"
	// technical peculiarities
	ARB_BUF_LIM = 512 * 1024 * 1024 // 512MiB
	// internal regular expressions
	IND_RE      = `^[ \t]*`
	YAML_IND_RE = `^[ \t]*(- )*`
	BLANK_RE    = `^[ \t]*$`
	RE_IGN_CASE = `(?i)`
	// default values
	DEF_PREFIX_DELIM = ":"
	DEF_SEPARATOR    = "--"
	DEF_STDIN_LABEL  = "(standard input)"
	// documentation
	DESC      = "prints indented text sections selected by matching a pattern."
	COPYRIGHT = `Copyright (C) 2019-2023 Erik Auerswald <auerswal@unix-ag.uni-kl.de>
License GPLv3+: GNU GPL version 3 or later <http://gnu.org/licenses/gpl.html>.
This is free software: you are free to change and redistribute it.
There is NO WARRANTY, to the extent permitted by law.`
	OD_BEGIN            = "also select all lines following first matched section"
	OD_ENCLOSING        = "select sections enclosing matched lines"
	OD_FIXED_STRING     = "PATTERN is fixed string, not regular expression"
	OD_HELP             = "display help text and exit"
	OD_IGNORE_BLANK     = "continue sections over blank lines"
	OD_IGNORE_CASE      = "ignore case distinctions"
	OD_IGNORE_RE        = "continue sections over lines matching regexp"
	OD_INDENT_RE        = "regular expression defining indentation"
	OD_INVERT_MATCH     = "match sections not starting with PATTERN"
	OD_LINE_NUMBER      = "prefix output lines with line number"
	OD_OMIT             = "omit (exclude) matched sections, print everything else"
	OD_OMIT_IGNORED     = "omit lines ignored as section breaks"
	OD_PREFIX_DELIM     = "string to delimit a prefix"
	OD_QUIET            = "suppress all normal output"
	OD_SEPARATOR        = "print a separator line between sections"
	OD_SEPARATOR_STRING = "specify separator string"
	OD_STDIN_LABEL      = "label in place of file name for standard input"
	OD_TAB_SIZE         = "number of characters between two tab stops"
	OD_TOP_LEVEL        = "sections start from minimum indentation level"
	OD_WITH_FILENAME    = "prefix output lines with file name"
	OD_YAML_IND         = "additionally allow YAML list indentation"
	OD_VERSION          = "display version and exit"
)

// parameterize section algorithm
type section_params struct {
	// options
	enclosing    bool
	fixed_string bool
	ignore_blank bool
	ignore_case  bool
	invert_match bool
	omit_ignored bool
	stdin_label  string
	tab_size     int
	top_level    bool
	yaml_ind     bool
	// regular expressions matching indentation
	ind_re *regexp.Regexp
	// regular expression matching lines to ignore
	ignore_re *regexp.Regexp
	// regular expression matching sections
	pat_re *regexp.Regexp
	// memory for processed lines
	memory line_memory
}

// line printer object
type line_printer struct {
	// state
	has_printed bool
	is_printing bool
	quiet       bool
	select_rest bool
	// values
	filename         string
	prefix_delim     string
	separator_string string
	// features
	begin         bool
	line_number   bool
	omit          bool
	separator     bool
	with_filename bool
}

// method to possibly print a line, depending on state and parameters
func (p *line_printer) print_line(l *[]byte, nr uint64, tr bool, is bool) (err error) {
	omit_selected := p.omit && (is || p.select_rest)
	omit_unselected := !p.omit && !(is || p.select_rest)
	is_transition := (tr || (!p.is_printing && p.omit)) && !p.select_rest
	if p.begin && is {
		p.select_rest = true
	}
	if p.quiet || omit_selected || omit_unselected {
		p.is_printing = false
		return nil
	}
	if p.separator && p.has_printed && is_transition {
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
	p.is_printing = true
	_, err = os.Stdout.WriteString("\n")
	return
}

// one line with added information
type line struct {
	l_ind int    // indentation level of this line
	s_ind int    // indentation level of section this line is in
	nr    uint64 // line number
	data  []byte // the bytes constituting the line itself
}

// interface to a collection of lines with added information
type line_memory interface {
	set_act(lp *line_printer)
	set_ign(lp *line_printer)
	add(l *[]byte, nr uint64, l_ind, s_ind int) (int, error)
	flush() (err error)
}

// a collection of lines with added information for a generic implementation
// of the simple ("memoryless") section algorithm
// (this is not a memoryless implementation)
type simple_line_memory struct {
	lines *[]line
	act   *line_printer // default output function
	ign   *line_printer // output function for ignored lines
}

// set the line printer for normal lines
func (lm *simple_line_memory) set_act(lp *line_printer) {
	lm.act = lp
}

// set the line printer for ignored lines
func (lm *simple_line_memory) set_ign(lp *line_printer) {
	lm.ign = lp
}

// add a line to the collection according to simple ("memoryless") rules for
// a generic implementation that does use extra memory to memorize lines
func (lm *simple_line_memory) add(l *[]byte, nr uint64, l_ind, s_ind int) (int, error) {
	// create a new data structure for the line
	new_line := line{
		l_ind: l_ind,
		s_ind: s_ind,
		nr:    nr,
	}
	new_line.data = make([]byte, len(*l))
	copy(new_line.data, *l)
	// ensure existence of lines slice to allow appending a line
	if lm.lines == nil {
		lm.lines = new([]line)
	}
	// append the line
	tmp := append(*lm.lines, new_line)
	lm.lines = &tmp
	// the simple ("memoryless") section algorithm does not adjust meta
	// data of previous lines, and does not adjust the section indentation
	// level
	return s_ind, nil
}

// print contents of a line collection and clear it
// this works identically for generic implementations of the "memoryless",
// "top level", and "enclosing" section algorithm variants
func (lm *simple_line_memory) flush() (err error) {
	prev_sect := false
	in_sect := false
	cont_sect := false
	new_sect := false
	if lm.lines == nil {
		return nil
	}
	for _, l := range *lm.lines {
		// ignore lines with unspecified indentation level
		if l.l_ind == -1 {
			err = lm.ign.print_line(&l.data, l.nr, false, in_sect)
			if err != nil {
				break
			}
			continue
		}
		in_sect = l.s_ind > -1
		cont_sect = in_sect && l.l_ind > l.s_ind
		new_sect = in_sect && (!cont_sect || !prev_sect)
		prev_sect = in_sect
		err = lm.act.print_line(&l.data, l.nr, new_sect, in_sect)
		if err != nil {
			break
		}
	}
	lm.lines = nil
	return
}

// memoryless implementation of simple ("memoryless") section algorithm
// this implementation differs from the generic one by not memorizing lines
type memoryless_lm struct {
	act *line_printer // default output function
	ign *line_printer // output function for ignored lines
}

// set the line printer for normal lines for memoryless implementation
func (lm *memoryless_lm) set_act(lp *line_printer) {
	lm.act = lp
}

// set the line printer for ignored lines for memoryless implementation
func (lm *memoryless_lm) set_ign(lp *line_printer) {
	lm.ign = lp
}

// the simple section algorithm can be implemented "memoryless", i.e.,
// without saving any lines, by just printing them
func (lm *memoryless_lm) add(l *[]byte, nr uint64, l_ind, s_ind int) (int, error) {
	var err error
	in_sect := s_ind > -1
	cont_sect := in_sect && l_ind > s_ind
	new_sect := in_sect && !cont_sect
	if l_ind == -1 {
		err = lm.ign.print_line(l, nr, false, in_sect)
	} else {
		err = lm.act.print_line(l, nr, new_sect, in_sect)
	}
	return s_ind, err
}

// nothing to do for "memoryless" implementation, but required to implement
// the line_memory interface
func (lm *memoryless_lm) flush() error {
	return nil
}

// a collection of lines with added information for the "top level"
// section algorithm
type top_level_lm struct {
	simple_line_memory
	matched bool
}

// use .set_act() method from generic implementation of the simple
// ("memoryless") section algorithm line memory for "top level"
func (lm *top_level_lm) set_act(lp *line_printer) {
	lm.simple_line_memory.act = lp
}

// use .set_ign() method from generic implementation of the simple
// ("memoryless") section algorithm line memory for "top level"
func (lm *top_level_lm) set_ign(lp *line_printer) {
	lm.simple_line_memory.ign = lp
}

// add a line to the collection according to "top level" section rules
func (lm *top_level_lm) add(l *[]byte, nr uint64, l_ind, s_ind int) (int, error) {
	var err error
	// as soon as the pattern has been matched, all lines can be sent
	// to the line printer instead of saving a copy for later
	if lm.matched {
		if l_ind == -1 {
			err = lm.ign.print_line(l, nr, false, true)
		} else {
			err = lm.act.print_line(l, nr, false, true)
		}
		return s_ind, err
	}
	// no pattern match yet, so save the line
	_, err = lm.simple_line_memory.add(l, nr, l_ind, s_ind)
	if err != nil {
		return s_ind, err
	}
	// ignored lines do not affect section meta data
	if l_ind == -1 {
		return s_ind, err
	}
	// only pattern match can affect section meta data
	if l_ind != s_ind {
		return s_ind, err
	}
	// all saved lines are part of the current section, and
	// the current section has the indentation level of the first
	// non-ignored line
	// all saved lines can be sent to the appropriate line printer marked
	// as "inside a section"
	lm.matched = true
	min_ind := -1
	var sl *line
	var new_sect bool
	for i := range *lm.lines {
		sl = &(*lm.lines)[i]
		// section has indentation level of first non-ignored line
		if sl.l_ind == -1 {
			err = lm.ign.print_line(&sl.data, sl.nr, false, true)
			if err != nil {
				if min_ind == -1 {
					return s_ind, err
				} else {
					break
				}
			}
			continue
		}
		if min_ind == -1 {
			min_ind = sl.l_ind
			new_sect = true
		}
		err = lm.act.print_line(&sl.data, sl.nr, new_sect, true)
		if err != nil {
			break
		}
		new_sect = false
	}
	// all saved lines have been sent to a line printer
	lm.lines = nil
	return min_ind, err
}

// the "top level" section algorithm variant allows simpler handling
// of saved lines than the generic .flush() implementation
func (lm *top_level_lm) flush() (err error) {
	// the last top level section is over, we do not have a match yet
	lm.matched = false
	if lm.lines == nil {
		return
	}
	// send all saved lines to the appropriate line printer marked as
	// "outside of a section"
	new_sect := true
	for _, l := range *lm.lines {
		if l.l_ind == -1 {
			err = lm.ign.print_line(&l.data, l.nr, false, false)
			if err != nil {
				break
			}
			continue
		}
		err = lm.act.print_line(&l.data, l.nr, new_sect, false)
		if err != nil {
			break
		}
		new_sect = false
	}
	lm.lines = nil
	return
}

// a collection of lines with added information for the "enclosing"
// section algorithm
type enclosing_lm struct {
	simple_line_memory
}

// use .set_act() method from the generic implementation of the simple
// ("memoryless") section algorithm line memory for "enclosing"
func (lm *enclosing_lm) set_act(lp *line_printer) {
	lm.simple_line_memory.act = lp
}

// use .set_ign() method from the generic implementation of the simple
// ("memoryless") section algorithm line memory for "enclosing"
func (lm *enclosing_lm) set_ign(lp *line_printer) {
	lm.simple_line_memory.ign = lp
}

// add a line to the collection according to "enclosing" section rules
func (lm *enclosing_lm) add(l *[]byte, nr uint64, l_ind, s_ind int) (int, error) {
	var err error
	_, err = lm.simple_line_memory.add(l, nr, l_ind, s_ind)
	if err != nil {
		return s_ind, err
	}
	// ignored lines do not affect section meta data
	if l_ind == -1 {
		return s_ind, err
	}
	// only pattern match can affect section meta data
	if l_ind != s_ind {
		return s_ind, err
	}
	// extend a new section from the last preceding line with lower
	// indentation level to the new line that was just added
	nr_lines := len(*lm.lines)
	// find section start
	i := nr_lines - 1
	for ; i > 0; i-- {
		if (*lm.lines)[i].l_ind != -1 && (*lm.lines)[i].l_ind < s_ind {
			break
		}
	}
	// determine section indentation level
	if (*lm.lines)[i].l_ind != -1 {
		s_ind = (*lm.lines)[i].l_ind
	} else {
		// line matching pattern starts the section
		return s_ind, err
	}
	// mark lines comprising section with newly found indentation level
	for ; i < nr_lines; i++ {
		if (*lm.lines)[i].l_ind != -1 {
			(*lm.lines)[i].s_ind = s_ind
		}
	}
	return s_ind, err
}

// use .flush() method from the generic implementation of the simple
// ("memoryless") section algorithm line memory for "enclosing"
func (lm *enclosing_lm) flush() (err error) {
	return lm.simple_line_memory.flush()
}

// print error with prefix
func print_err(err error) {
	log.SetPrefix(PROG + ": error: ")
	log.Print(err)
}

// print short usage information
func usage(w io.Writer) {
	fmt.Fprintf(w, "Usage: %s [OPTION...] PATTERN [FILE...]\n", PROG)
}

// print error with prefix, print short usage info, then exit with code 2
func usage_err(err error) {
	log.SetPrefix(PROG + ": error: ")
	log.Print(err)
	usage(os.Stderr)
	fmt.Fprintf(os.Stderr, "Try '%s --help' for more information\n", PROG)
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

// compute indentation depth from indentation byte sequence
func indentation_depth(in *[]byte, ts int) int {
	if in == nil {
		return 0
	}
	if ts < 2 {
		return len(*in)
	}
	d := 0
	for _, c := range *in {
		if c == '\t' {
			d += ts - (d % ts)
		} else {
			d++
		}
	}
	return d
}

// read input text and write matching sections to output
func section(p section_params, r io.Reader) (matched bool, err error) {
	matched = false    // return if something was matched
	err = nil          // return an error, if one occurs
	in_sect := false   // currently inside a section?
	cont_sect := false // continue the current section?
	pat_match := false // does current line match pattern?
	s_ind := -1        // indentation depth of current section
	c_ind := -1        // indentation depth of current line
	min_ind := -1      // minimal indentation level seen so far
	var buf []byte     // buffer space to hold input data
	var l []byte       // one line of input data
	var li []byte      // indentation bytes of the line
	var l_nr uint64    // current line number

	// process input line by line
	s := bufio.NewScanner(r)
	s.Buffer(buf, ARB_BUF_LIM)
	for s.Scan() {
		l_nr++
		l = s.Bytes()
		// ignored lines do not cause a section transition
		if p.ignore_re != nil && p.ignore_re.Match(l) {
			_, err = p.memory.add(&l, l_nr, -1, s_ind)
			if err != nil {
				print_err(err)
				return
			}
			continue
		}
		// determine indentation depth of current line
		li = p.ind_re.Find(l)
		c_ind = indentation_depth(&li, p.tab_size)
		// manage top level section status
		if min_ind > -1 && c_ind <= min_ind {
			// print a completed top level section
			min_ind = c_ind
			err = p.memory.flush()
			if err != nil {
				print_err(err)
				return
			}
		} else if min_ind == -1 {
			// initialize top level indentation
			min_ind = c_ind
		}
		// check if current line matches pattern
		pat_match = p.pat_re.Match(l)
		if p.invert_match {
			pat_match = !pat_match
		}
		// is the current line a continuation of a section?
		cont_sect = in_sect && (c_ind > s_ind)
		if !cont_sect {
			if pat_match {
				matched = true
				in_sect = true
				s_ind = c_ind
			} else {
				in_sect = false
				s_ind = -1
			}
		}
		// add current line to memory
		s_ind, err = p.memory.add(&l, l_nr, c_ind, s_ind)
		if err != nil {
			print_err(err)
			return
		}
	}
	// print last top level section
	err = p.memory.flush()
	if err != nil {
		print_err(err)
	}
	err = s.Err()
	if err != nil {
		print_err(err)
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
	}
	// default line printer
	lp := line_printer{
		separator_string: DEF_SEPARATOR,
		filename:         "",
	}

	// error logging
	log.SetPrefix(PROG + ": ")
	log.SetFlags(0)

	// define command line flags
	flag.Usage = func() { usage_err(errors.New("unknown option")) }
	// print program information instead of sections
	var print_help, print_version bool
	flag.BoolVar(&print_help, "help", false, OD_HELP)
	flag.BoolVar(&print_help, "h", false, OD_HELP)
	flag.BoolVar(&print_version, "version", false, OD_VERSION)
	flag.BoolVar(&print_version, "V", false, OD_VERSION)
	// modify section behavior
	var ignore_re, indent_re string
	flag.BoolVar(&lp.begin, "begin", false, OD_BEGIN)
	flag.BoolVar(&sp.enclosing, "enclosing", false, OD_ENCLOSING)
	flag.BoolVar(&sp.fixed_string, "fixed-string", false, OD_FIXED_STRING)
	flag.BoolVar(&sp.fixed_string, "F", false, OD_FIXED_STRING)
	flag.BoolVar(&sp.ignore_case, "ignore-case", false, OD_IGNORE_CASE)
	flag.BoolVar(&sp.ignore_case, "i", false, OD_IGNORE_CASE)
	flag.BoolVar(&sp.ignore_blank, "ignore-blank", false, OD_IGNORE_BLANK)
	flag.StringVar(&ignore_re, "ignore-re", "", OD_IGNORE_RE)
	flag.StringVar(&indent_re, "indent-re", IND_RE, OD_INDENT_RE)
	flag.BoolVar(&sp.invert_match, "invert-match", false, OD_INVERT_MATCH)
	flag.StringVar(&sp.stdin_label, "label", DEF_STDIN_LABEL,
		OD_STDIN_LABEL)
	flag.BoolVar(&lp.line_number, "line-number", false, OD_LINE_NUMBER)
	flag.BoolVar(&lp.line_number, "n", false, OD_LINE_NUMBER)
	flag.BoolVar(&lp.omit, "omit", false, OD_OMIT)
	flag.BoolVar(&sp.omit_ignored, "omit-ignored", false, OD_OMIT_IGNORED)
	flag.StringVar(&lp.prefix_delim, "prefix-delimiter", DEF_PREFIX_DELIM,
		OD_PREFIX_DELIM)
	flag.BoolVar(&lp.quiet, "quiet", false, OD_QUIET)
	flag.BoolVar(&lp.quiet, "q", false, OD_QUIET)
	flag.BoolVar(&lp.quiet, "silent", false, OD_QUIET)
	flag.BoolVar(&lp.separator, "separator", false, OD_SEPARATOR)
	flag.StringVar(&lp.separator_string, "separator-string", DEF_SEPARATOR,
		OD_SEPARATOR_STRING)
	flag.IntVar(&sp.tab_size, "tab-size", 8, OD_TAB_SIZE)
	flag.BoolVar(&sp.top_level, "top-level", false, OD_TOP_LEVEL)
	flag.BoolVar(&lp.with_filename, "with-filename", false,
		OD_WITH_FILENAME)
	flag.BoolVar(&sp.yaml_ind, "yaml", false, OD_YAML_IND)
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
	// section parameters
	if sp.ignore_blank {
		sp.ignore_re = regexp.MustCompile(BLANK_RE)
	} else if ignore_re != "" {
		sp.ignore_re, err = regexp.Compile(ignore_re)
		if err != nil {
			print_err(err)
			usage_err(errors.New("invalid --ignore-re argument"))
		}
	}
	if sp.yaml_ind {
		sp.ind_re = regexp.MustCompile(YAML_IND_RE)
	} else {
		sp.ind_re, err = regexp.Compile(indent_re)
		if err != nil {
			print_err(err)
			usage_err(errors.New("invalid --indent-re argument:"))
		}
	}
	// line memory selection
	if sp.top_level {
		sp.memory = new(top_level_lm)
	} else if sp.enclosing {
		sp.memory = new(enclosing_lm)
	} else {
		sp.memory = new(memoryless_lm)
	}
	// already parameterized line printer as normal action
	sp.memory.set_act(&lp)
	// "ignore" line printer may be different from normal one
	if sp.omit_ignored {
		no_output := line_printer{
			quiet: true,
		}
		sp.memory.set_ign(&no_output)
	} else {
		sp.memory.set_ign(&lp)
	}
	// required pattern to match on is given as command line argument
	if flag.NArg() < 1 {
		usage_err(errors.New("PATTERN is missing"))
	}
	pat_str := flag.Arg(0)
	// escape meta characters if PATTERN is intended as a fixed string
	if sp.fixed_string {
		pat_str = regexp.QuoteMeta(pat_str)
	}
	// adjust pattern according to command line flags
	if sp.ignore_case {
		pat_str = RE_IGN_CASE + pat_str
	}
	sp.pat_re, err = regexp.Compile(pat_str)
	if err != nil {
		print_err(err)
		usage_err(errors.New("invalid PATTERN"))
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
			m := false
			f, err := os.Open(arg)
			if err != nil {
				print_err(err)
				ec = exit_code(ec, m, err)
				continue
			}
			lp.filename = arg
			if lp.begin {
				lp.select_rest = false
			}
			m, err = section(sp, f)
			ec = exit_code(ec, m, err)
			f.Close()
		}
	}
	os.Exit(ec)
}
