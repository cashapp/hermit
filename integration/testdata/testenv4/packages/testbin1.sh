#!/usr/bin/env bash

# In the environment for this package, there is:
# - a FOO from package `other` with value `otherfoo`
# - a FOO from package `runtimedep` with value `runtimefoo`
# The last one (runtimefoo) should override the other one
echo "FOO=${FOO}"
