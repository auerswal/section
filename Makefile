SOURCE  := section.go
BINARY  := section
MAN     := $(BINARY).1
MANSRC  := $(MAN).in
MANWEB  := $(MAN).html
TESTDIR := tests
TESTBIN := $(TESTDIR)/run_tests
TESTS   := $(wildcard $(TESTDIR)/*.ec $(TESTDIR)/*.exp $(TESTDIR)/*.in $(TESTDIR)/*.in.? $(TESTDIR)/*.opts $(TESTDIR)/*.pat)
HELPERS := generate_man_page_date.sh
PREFIX  := /usr/local
BINDIR  := $(PREFIX)/bin
MANDIR  := $(PREFIX)/share/man/man1
DOCDIR  := $(PREFIX)/share/doc/$(BINARY)
DOCS    := COPYING README INSTALL NEWS
VERSION := $(shell sed -En 's/^.*VERSION.*=.*"([0-9]+(\.[0-9]+){2}\+?)".*$$/\1/p' section.go)
CRYEARS := $(shell sed -En 's/^ +Copyright[^0-9]+([0-9]+(-[0-9]+)?) .*$$/\1/p' section.go)
SRCDIR  := $(BINARY)-$(VERSION)
ALLSRC  := Makefile $(SOURCE) $(MANSRC) $(DOCS) $(MAN)
ARCHIVE := $(SRCDIR).tar.gz
GC      := $(if $(shell which gccgo),gccgo -static,go build)

all: $(BINARY) $(MAN)

$(BINARY): $(SOURCE) Makefile
	$(GC) -o $@ $<

$(MAN): $(MANSRC) Makefile generate_man_page_date.sh section.go NEWS
	sed -e 's/@VERSION@/$(VERSION)/' \
	    -e 's/@DATE@/$(shell ./generate_man_page_date.sh)/' \
	    -e 's/@CRYEARS@/$(CRYEARS)/' <$< >$@

$(MANWEB): $(MAN) Makefile
	man -l -Thtml $< >$@

check: $(BINARY)
	(cd tests; ./run_tests)

install: all $(DOCS)
	install -d $(DESTDIR)$(BINDIR) $(DESTDIR)$(MANDIR) $(DESTDIR)$(DOCDIR)
	install -m 0755 $(BINARY) $(DESTDIR)$(BINDIR)/$(BINARY)
	install -m 0644 $(MAN) $(DESTDIR)$(MANDIR)/$(MAN)
	gzip -9 $(DESTDIR)$(MANDIR)/$(MAN)
	install -m 0644 $(DOCS) $(DESTDIR)$(DOCDIR)/

$(SRCDIR): $(SOURCE) $(DOCS) $(MAN) $(HELPERS) $(TESTS) Makefile
	install -d $(SRCDIR)/$(TESTDIR)
	install -m 0644 $(ALLSRC) $(SRCDIR)/
	install -m 0644 $(TESTS) $(SRCDIR)/$(TESTDIR)/
	install -m 0755 $(TESTBIN) $(SRCDIR)/$(TESTDIR)/
	install -m 0755 $(HELPERS) $(SRCDIR)/

tar: $(SRCDIR)
	tar cvfz $(ARCHIVE) $(SRCDIR)

clean:
	$(RM) $(BINARY) $(MANWEB)
	$(RM) -r $(SRCDIR)
	$(RM) $(wildcard tests/*.out) tests/tests.log

distclean: clean
	$(RM) $(MAN) $(wildcard $(BINARY)-*.*.*.tar.gz)

.PHONY: check clean distclean install
