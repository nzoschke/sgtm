.PHONY: build scan inspect dashboard clean

BINARY := bin/sgtm
PLIST := cmd/sgtm/Info.plist
GOFLAGS ?=

build:
	mkdir -p bin
	go build $(GOFLAGS) -ldflags '-linkmode external -extldflags "-Wl,-sectcreate,__TEXT,__info_plist,$(PLIST)"' -o $(BINARY) ./cmd/sgtm
	codesign --force --sign - --identifier com.nzoschke.sgtm $(BINARY)

scan: build
	$(BINARY) scan --duration 20s

inspect: build
	$(BINARY) inspect $(ARGS)

dashboard: build
	$(BINARY) dashboard $(ARGS)

clean:
	rm -rf bin
