# Hermit {{.Shell}} activation script

set -gx HERMIT_ENV {{.Root}}

if set -q ACTIVE_HERMIT
    if test "$ACTIVE_HERMIT" = "$HERMIT_ENV"
        if functions -q deactivate-hermit
            echo "This Hermit environment has already been activated. Skipping" >&2
            return 0
        end
    else if functions -q deactivate-hermit
        set -lx HERMIT_CURRENT_ENV $HERMIT_ENV
        set -gx HERMIT_ENV $ACTIVE_HERMIT
        deactivate-hermit
        set -gx HERMIT_ENV $HERMIT_CURRENT_ENV
    else
        set -l HERMIT_CURRENT_ENV $HERMIT_ENV
        set -gx HERMIT_ENV $ACTIVE_HERMIT
        "$ACTIVE_HERMIT/bin/hermit" env --deactivate-from-ops="$HERMIT_ENV_OPS" | source
        set -gx HERMIT_ENV $HERMIT_CURRENT_ENV
    end
end

{{ range $ENV_NAME, $ENV_VALUE := .Env }}
set -gx {{ $ENV_NAME }} {{ $ENV_VALUE | Quote }}
{{- end }}

function _hermit_deactivate
    echo "Hermit environment $($HERMIT_ENV/bin/hermit env HERMIT_ENV) deactivated"
    "$ACTIVE_HERMIT/bin/hermit" env --deactivate-from-ops="$HERMIT_ENV_OPS" | source
    functions -e deactivate-hermit > /dev/null 2>&1
    functions -e update_hermit_env > /dev/null 2>&1
    set -e ACTIVE_HERMIT
    set -e HERMIT_ENV_OPS

    # Clear the command cache
    functions -c > /dev/null
end

# Wrapper function for deactivating Hermit
function deactivate-hermit
    set -gx DEACTIVATED_HERMIT "$HERMIT_ENV"
    _hermit_deactivate
end

# Initialize the Hermit environment
set -e DEACTIVATED_HERMIT
set -gx ACTIVE_HERMIT $HERMIT_ENV
set -gx HERMIT_ENV_OPS $("$HERMIT_ENV/bin/hermit" env --ops)
set -gx HERMIT_BIN_CHANGE $(date -r "$HERMIT_ENV/bin" +"%s")

# Function to update Hermit environment
function update_hermit_env
    set CURRENT $(date -r "$HERMIT_ENV/bin" +"%s")
    test "$CURRENT" = "$HERMIT_BIN_CHANGE"; and return 0
    set CUR_HERMIT "$HERMIT_ENV/bin/hermit"
    "$ACTIVE_HERMIT/bin/hermit" env --deactivate-from-ops="$HERMIT_ENV_OPS" | source
    "$CUR_HERMIT" env --activate | source
    set -gx HERMIT_ENV_OPS $("$HERMIT_ENV/bin/hermit" env --ops)
    set -gx HERMIT_BIN_CHANGE $CURRENT
end
