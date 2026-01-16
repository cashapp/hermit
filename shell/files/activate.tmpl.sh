# Hermit {{.Shell}} activation script

HERMIT_TARGET={{.Root}}

if [ -n "${ACTIVE_HERMIT+_}" ]; then
  if [ "$ACTIVE_HERMIT" = "$HERMIT_TARGET" ]; then
    if [ -n "${_HERMIT_SHELL_ACTIVE+_}" ] && [ "${_HERMIT_SHELL_ACTIVE}" -ef "${HERMIT_TARGET}" ]; then
      echo "This Hermit environment has already been activated. Skipping" >&2
      unset HERMIT_TARGET
      return 0
    fi
  elif type deactivate-hermit &>/dev/null; then
    HERMIT_ENV=$ACTIVE_HERMIT
    deactivate-hermit
  else
    HERMIT_ENV=$ACTIVE_HERMIT
    eval "$(${ACTIVE_HERMIT}/bin/hermit env --deactivate-from-ops="${HERMIT_ENV_OPS}")"
  fi
fi

export HERMIT_ENV=$HERMIT_TARGET
unset HERMIT_TARGET

{{ range $ENV_NAME, $ENV_VALUE := .Env }}
export {{ $ENV_NAME }}={{ $ENV_VALUE | Quote }}
{{ end }}

_hermit_deactivate() {
  echo "Hermit environment $(${HERMIT_ENV}/bin/hermit env HERMIT_ENV) deactivated"
  eval "$(${ACTIVE_HERMIT}/bin/hermit env --deactivate-from-ops="${HERMIT_ENV_OPS}")"
  unset -f deactivate-hermit >/dev/null 2>&1
  unset -f update_hermit_env >/dev/null 2>&1
  unset ACTIVE_HERMIT
  unset HERMIT_ENV_OPS
  unset _HERMIT_SHELL_ACTIVE

  hash -r 2>/dev/null

{{- if .Bash }}
  unset PROMPT_COMMAND >/dev/null 2>&1
  if test -n "${_HERMIT_OLD_PROMPT_COMMAND+_}"; then PROMPT_COMMAND="${_HERMIT_OLD_PROMPT_COMMAND}"; unset _HERMIT_OLD_PROMPT_COMMAND; fi
{{- end}}

{{- if .Zsh }}
  precmd_functions=(${precmd_functions:#update_hermit_env})
{{- end}}

{{- if ne .Prompt "none"}}
  if test -n "${_HERMIT_OLD_PS1+_}"; then export PS1="${_HERMIT_OLD_PS1}"; unset _HERMIT_OLD_PS1; fi
{{- end}}

}

deactivate-hermit() {
  export DEACTIVATED_HERMIT="$HERMIT_ENV"
  _hermit_deactivate
}


unset DEACTIVATED_HERMIT
export ACTIVE_HERMIT=$HERMIT_ENV
export HERMIT_ENV_OPS="$(${HERMIT_ENV}/bin/hermit env --ops)"
export HERMIT_BIN_CHANGE=$(date -r ${HERMIT_ENV}/bin +"%s")
_HERMIT_SHELL_ACTIVE=$HERMIT_ENV

{{- if ne .Prompt "none" }}
if test -n "${PS1+_}"; then export _HERMIT_OLD_PS1="${PS1}"; PS1="{{if eq .Prompt "env"}}{{ .EnvName }}{{end}}🐚 ${PS1}"; fi
{{- end}}

update_hermit_env() {
  local CURRENT=$(date -r ${HERMIT_ENV}/bin +"%s")
  test "$CURRENT" = "$HERMIT_BIN_CHANGE" && return 0
  local CUR_HERMIT=${HERMIT_ENV}/bin/hermit
  eval "$(${ACTIVE_HERMIT}/bin/hermit env --deactivate-from-ops="${HERMIT_ENV_OPS}")"
  eval "$(${CUR_HERMIT} env --activate)"
  export HERMIT_ENV_OPS=$(${HERMIT_ENV}/bin/hermit env --ops)
  export HERMIT_BIN_CHANGE=$CURRENT
}

{{- if .Bash }}
if test -n "${PROMPT_COMMAND+_}"; then
  _HERMIT_OLD_PROMPT_COMMAND="${PROMPT_COMMAND}"
  PROMPT_COMMAND="update_hermit_env; $PROMPT_COMMAND"
else
  PROMPT_COMMAND="update_hermit_env"
fi
{{- end}}

{{- if .Zsh }}
precmd_functions+=(update_hermit_env)
{{- end}}
