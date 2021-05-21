set -eu

# Wrap calls to run the command hooks with the call
# Normally this is done by shell hooks, but shellspec does not support them.
prompt_hooks() {
  $@
  res=$?
  # We need to reset the change timestamp, as file timestamps are at second resolution.
  # Some IT updates could be lost without this
  export HERMIT_BIN_CHANGE=0

  if test -n "${PROMPT_COMMAND+_}"; then
    eval "$PROMPT_COMMAND"
  elif [ -n "${ZSH_VERSION-}" ]; then
    update_hermit_env
  fi

  return $res
}

clear_state() {
  chmod -f -R u+w ../state 2> /dev/null || true
  rm -rf ../state
}