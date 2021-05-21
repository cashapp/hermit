.PHONY: dev
dev: schema
	../bin/hugo server -D

.PHONY: build
build: schema
	../bin/hugo

.PHONY: schema
schema:
	(cd .. && ./bin/go run ./cmd/gendocs ./docs/content/packaging/schema/)
