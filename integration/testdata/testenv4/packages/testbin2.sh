#!/usr/bin/env bash

# In the environment for this package, there is:
# - a BAR from package `other` with value `otherbar`
# - a BAR from package `runtimedep` with value `runtimebar`
# - a BAR from the hermit.hcl with value `hermitbar`
# The last one (hermitbar) should override the other two
echo "BAR=${BAR}"
