---
title: "Bundling"
---

A bundle is a completely self-contained directory of packages and associated environment variables generated from an
existing Hermit environment. It has no dependency on Hermit. It can be useful for ensuring production packages used in
Docker containers are identical to the packages used in development, for example.

## Example

Create a new environment and bundle from it:

```console
$ hermit init .
$ hermit install go
$ hermit bundle ../dist
info: Created exploded bundle:
info:   Root: /Users/aat/dev/dist
info:    Bin: /Users/aat/dev/dist/bin
info:   .env: /Users/aat/dev/dist/.env
info:         GOROOT="${PWD}/go-1.24.5"
info:         GOTOOLCHAIN="local"
info:         PATH="${PWD}/.hermit/go/bin:${PWD}/bin:$PATH"
info:         GOBIN="${PWD}/.hermit/go/bin"
```

The layout of the bundle will look something like this:

```console
$ cd ../dist
$ ls -l bin
total 0
lrwxr-xr-x  1 aat  staff  19 27 Aug 19:36 go@ -> ../go-1.24.5/bin/go
lrwxr-xr-x  1 aat  staff  22 27 Aug 19:36 gofmt@ -> ../go-1.24.5/bin/gofmt
$ cat .env
GOROOT="${PWD}/go-1.24.5"
GOTOOLCHAIN="local"
PATH="${PWD}/.hermit/go/bin:${PWD}/bin:$PATH"
GOBIN="${PWD}/.hermit/go/bin"
```

You can then "activate" the bundle by sourcing the `.env` file:

```console
$ . .env
$ echo $GOROOT
/Users/aat/dev/dist/go-1.24.5
```
