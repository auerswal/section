SOURCE  := section.go
BINARY  := section
MAN     := $(BINARY).1
MANSRC  := $(MAN).in
PREFIX  := /usr/local
BINDIR  := $(PREFIX)/bin
MANDIR  := $(PREFIX)/share/man/man1
DOCDIR  := $(PREFIX)/share/doc/$(BINARY)
DOCS    := COPYING README
VERSION := $(shell sed -En 's/^.*VERSION.*=.*"([0-9]+(\.[0-9]+){2})".*$$/\1/p' section.go)
CRYEARS := $(shell sed -En 's/^ +Copyright[^0-9]+([0-9]+(-[0-9]+)?) .*$$/\1/p' section.go)
SRCDIR  := $(BINARY)-$(VERSION)
ALLSRC  := Makefile $(SOURCE) $(MANSRC) $(DOCS)
ARCHIVE := $(SRCDIR).tar.gz
GC      := $(if $(shell which gccgo),gccgo,go build)

all: $(BINARY) $(MAN)

$(BINARY): $(SOURCE) Makefile
	$(GC) -o $@ $<

$(MAN): $(MANSRC) Makefile
	sed -e 's/@VERSION@/$(VERSION)/' \
	    -e "s/@DATE@/$(shell date +%Y-%m-%d)/" \
	    -e 's/@CRYEARS@/$(CRYEARS)/' <$< >$@

check: $(BINARY)
	(cd tests; ./run_tests)

install: all $(DOCS)
	install -d $(DESTDIR)$(BINDIR) $(DESTDIR)$(MANDIR) $(DESTDIR)$(DOCDIR)
	install -m 0755 $(BINARY) $(DESTDIR)$(BINDIR)/$(BINARY)
	install -m 0644 $(MAN) $(DESTDIR)$(MANDIR)/$(MAN)
	gzip -9 $(DESTDIR)$(MANDIR)/$(MAN)
	install -m 0644 $(DOCS) $(DESTDIR)$(DOCDIR)/

$(SRCDIR): $(SOURCE) $(DOCS) Makefile
	install -d $(SRCDIR)
	install -m 0644 $(ALLSRC) $(SRCDIR)/

tar: $(SRCDIR)
	tar cvfz $(ARCHIVE) $(SRCDIR)

clean:
	$(RM) $(BINARY) $(MAN)
	$(RM) -r $(SRCDIR)
	$(RM) $(wildcard tests/*.out) tests/tests.log

distclean: clean
	$(RM) $(ARCHIVE)

.PHONY: check clean install
