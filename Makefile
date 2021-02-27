SOURCE  := section.go
BINARY  := section
PREFIX  := /usr/local
BINDIR  := $(PREFIX)/bin
DOCDIR  := $(PREFIX)/share/doc/$(BINARY)
DOCS    := COPYING README
VERSION := $(shell sed -En 's/^.*VERSION.*=.*"([0-9]+(\.[0-9]+){2})".*$$/\1/p' section.go)
SRCDIR  := $(BINARY)-$(VERSION)
ALLSRC  := Makefile $(SOURCE) $(DOCS)
ARCHIVE := $(SRCDIR).tar.gz
GC      := $(if $(shell which gccgo),gccgo,go build)

all: $(BINARY)

$(BINARY): $(SOURCE) Makefile
	$(GC) -o $@ $<

check: $(BINARY)
	(cd tests; ./run_tests)

install: all $(DOCS)
	install -d $(DESTDIR)$(BINDIR) $(DESTDIR)$(DOCDIR)
	install -m 0755 $(BINARY) $(DESTDIR)$(BINDIR)/$(BINARY)
	install -m 0644 $(DOCS) $(DESTDIR)$(DOCDIR)/

$(SRCDIR): $(SOURCE) $(DOCS) Makefile
	install -d $(SRCDIR)
	install -m 0644 $(ALLSRC) $(SRCDIR)/

tar: $(SRCDIR)
	tar cvfz $(ARCHIVE) $(SRCDIR)

clean:
	$(RM) $(BINARY)
	$(RM) -r $(SRCDIR)
	$(RM) $(wildcard tests/*.out) tests/tests.log

distclean: clean
	$(RM) $(ARCHIVE)

.PHONY: check clean install
