change_hermit_env() {
  local CUR=${PWD}
  while [ "$CUR" != "/" ]; do
    if [ -n "${HERMIT_ENV+_}" ] && [ "${CUR}" -ef "${HERMIT_ENV}" ]; then
      if [ -n "${DEACTIVATED_HERMIT+_}" ] && [ "${CUR}" -ef "${DEACTIVATED_HERMIT}" ]; then
        return
      fi
      if typeset -f deactivate-hermit >/dev/null 2>&1; then
        return
      fi
    fi
    if [ -f "${CUR}/bin/activate-hermit" ]; then
      if [ -n "${HERMIT_ENV+_}"  ]; then type _hermit_deactivate &>/dev/null && _hermit_deactivate; fi
      # shellcheck source=files/activate-hermit
      if [ -z "${DEACTIVATED_HERMIT+_}" ] || ! [ "${CUR}" -ef "${DEACTIVATED_HERMIT}" ]; then
        if "${HERMIT_ROOT_BIN:-"$HOME/bin/hermit"}" --quiet validate env "${CUR}"; then
          . "${CUR}/bin/activate-hermit"
        fi
      fi
      return
    fi
    CUR="$(dirname "${CUR}")"
  done
  unset DEACTIVATED_HERMIT
  if [ -n "${HERMIT_ENV+_}"  ]; then type _hermit_deactivate &>/dev/null && _hermit_deactivate; fi
}
