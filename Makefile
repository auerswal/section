SOURCE  := section.go
BINARY  := section
MAN     := $(BINARY).1
MANDATE := $(shell git log -n1 --date=short section.1.in | awk '/^Date:/ { print $$2 }')
MANSRC  := $(MAN).in
MANWEB  := $(MAN).html
PREFIX  := /usr/local
BINDIR  := $(PREFIX)/bin
MANDIR  := $(PREFIX)/share/man/man1
DOCDIR  := $(PREFIX)/share/doc/$(BINARY)
DOCS    := COPYING README INSTALL
VERSION := $(shell sed -En 's/^.*VERSION.*=.*"([0-9]+(\.[0-9]+){2})".*$$/\1/p' section.go)
CRYEARS := $(shell sed -En 's/^ +Copyright[^0-9]+([0-9]+(-[0-9]+)?) .*$$/\1/p' section.go)
SRCDIR  := $(BINARY)-$(VERSION)
ALLSRC  := Makefile $(SOURCE) $(DOCS) $(MAN)
ARCHIVE := $(SRCDIR).tar.gz
GC      := $(if $(shell which gccgo),gccgo,go build)

all: $(BINARY) $(MAN)

$(BINARY): $(SOURCE) Makefile
	$(GC) -o $@ $<

$(MAN): $(MANSRC) Makefile
	sed -e 's/@VERSION@/$(VERSION)/' \
	    -e 's/@DATE@/$(MANDATE)/' \
	    -e 's/@CRYEARS@/$(CRYEARS)/' <$< >$@

$(MANWEB): $(MAN) Makefile
	mandoc -T html $< >$@

check: $(BINARY)
	(cd tests; ./run_tests)

install: all $(DOCS)
	install -d $(DESTDIR)$(BINDIR) $(DESTDIR)$(MANDIR) $(DESTDIR)$(DOCDIR)
	install -m 0755 $(BINARY) $(DESTDIR)$(BINDIR)/$(BINARY)
	install -m 0644 $(MAN) $(DESTDIR)$(MANDIR)/$(MAN)
	gzip -9 $(DESTDIR)$(MANDIR)/$(MAN)
	install -m 0644 $(DOCS) $(DESTDIR)$(DOCDIR)/

$(SRCDIR): $(SOURCE) $(DOCS) $(MAN) Makefile
	install -d $(SRCDIR)
	install -m 0644 $(ALLSRC) $(SRCDIR)/

tar: $(SRCDIR)
	tar cvfz $(ARCHIVE) $(SRCDIR)

clean:
	$(RM) $(BINARY) $(MAN) $(MANWEB)
	$(RM) -r $(SRCDIR)
	$(RM) $(wildcard tests/*.out) tests/tests.log

distclean: clean
	$(RM) $(ARCHIVE)

.PHONY: check clean install
