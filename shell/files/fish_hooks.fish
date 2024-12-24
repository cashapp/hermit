function activate_hermit --on-variable PWD
  set CUR $PWD
  set -q HERMIT_ROOT_BIN; or set HERMIT_ROOT_BIN "$HOME/bin/hermit"

  while test "$CUR" != "/"
      if test "$CUR" -ef "$HERMIT_ENV"
          return
      end
      if test -f "$CUR/bin/activate-hermit.fish"
          if test -n "$HERMIT_ENV"
              type -q _hermit_deactivate; and _hermit_deactivate
          end
          # Validate and activate the Hermit environment
          if not test "$CUR" -ef "$DEACTIVATED_HERMIT"
              if "$HERMIT_ROOT_BIN" --quiet validate env "$CUR" >/dev/null 2>&1
                  source "$CUR/bin/activate-hermit.fish"
              end
          end
          return
      end
      set CUR (dirname "$CUR")
  end
  set -e DEACTIVATED_HERMIT
  if test -n "$HERMIT_ENV"
      type -q _hermit_deactivate; and _hermit_deactivate
  end
end
