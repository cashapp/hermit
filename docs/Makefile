.PHONY: dev
dev: schema
	../bin/hugo server -D

.PHONY: build
build: schema
	../bin/hugo

.PHONY: schema
schema:
	(cd .. && ./bin/go run ./cmd/gendocs ./docs/content/packaging/schema/)
	go run ../cmd/hermit dump-user-config-schema | sed 's,//,#,g' > ./content/usage/user-config-schema.hcl
