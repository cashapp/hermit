change_hermit_env() {
  local CUR=${PWD}
  while [ "$CUR" != "/" ]; do
    if [ "${CUR}" -ef "${HERMIT_ENV}" ]; then return; fi
    if [ -f "${CUR}/bin/activate-hermit" ]; then
      if [ -n "${HERMIT_ENV+_}"  ]; then type _hermit_deactivate &>/dev/null && _hermit_deactivate; fi
      # shellcheck source=files/activate-hermit
      if ! [ "${CUR}" -ef "${DEACTIVATED_HERMIT}" ]; then
        if "${HOME}/bin/hermit" --quiet validate env "${CUR}"; then
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
