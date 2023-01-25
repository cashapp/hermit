.PHONY: dev
dev: schema
	mkdocs serve

.PHONY: schema
schema:
	(cd .. && ./bin/go run ./cmd/gendocs ./docs/docs/packaging/schema/)
	go run ../cmd/hermit dump-user-config-schema | sed 's,//,#,g' > docs/usage/user-config-schema.hcl
	