function activate_hermit --on-variable PWD
  set CUR $PWD
  set -q HERMIT_ROOT_BIN; or set HERMIT_ROOT_BIN "$HOME/bin/hermit"

  while test "$CUR" != "/"
      if set -q HERMIT_ENV; and test "$CUR" -ef "$HERMIT_ENV"
          if set -q DEACTIVATED_HERMIT; and test "$CUR" -ef "$DEACTIVATED_HERMIT"
              return
          end
          if functions -q deactivate-hermit
              return
          end
      end
      if test -f "$CUR/bin/activate-hermit.fish"
          if test -n "$HERMIT_ENV"
              type -q _hermit_deactivate; and _hermit_deactivate
          end
          # Validate and activate the Hermit environment
          if not set -q DEACTIVATED_HERMIT; or not test "$CUR" -ef "$DEACTIVATED_HERMIT"
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

function __complete_hermit
    set -q HERMIT_ROOT_BIN; or set HERMIT_ROOT_BIN "$HOME/bin/hermit"
    set -lx COMP_LINE (commandline -cp)
    test -z (commandline -ct)
    and set COMP_LINE "$COMP_LINE "
    $HERMIT_ROOT_BIN noop
end

complete -f -c hermit -a "(__complete_hermit)"
