Describe "Hermit"
  cd testenv

  Describe "Initialisation"
    It "runs successfully"
      When run command hermit init .
      The status should be success
      The stdout should be blank
      The stderr should be blank
    End
    It "creates hermit proxy in ./bin/hermit"
      The file ./bin/hermit should be exist
    End
    It "creates hermit activation script in ./bin/activate-hermit"
      The file ./bin/activate-hermit should be exist
    End
  End

  Describe "Activating the environment"
      It "sets the environment correctly"
        When call source ./bin/activate-hermit
        The status should be success
        The stdout should not be blank
        The stderr should not be blank
        The variable HERMIT_ENV should equal "$(pwd)"
      End
      It "fails to activate twice"
        . ./bin/activate-hermit
        When call source ./bin/activate-hermit
        The status should be failure
        The stderr should eq "This Hermit environment has already been activated. Skipping"
      End
      It "loads environment variables from the local hermit.hcl"
        When call source ./bin/activate-hermit
        The stdout should not be blank
        The variable GOBIN should equal "$(pwd)/out/bin"
      End
  End

  Describe "Interacting with the active environment"
    Before "export PKG_TEST_VAR=before"
    Before ". ./bin/activate-hermit"

    Describe "Installing a new package"
      It "places the symlinks in the environment /bin"
        When call prompt_hooks hermit install protoc-3.7.1
        The status should be success
        The stderr should be blank
        The file ./bin/protoc should be exist
        The file ./bin/.protoc-3.7.1.pkg should be exist
        The variable PKG_TEST_VAR should equal "=test\"value\""
      End
    End

    Describe "Upgrading a new package"
      It "updates the symlinks and environment variables in the environment"
        When call prompt_hooks hermit upgrade protoc
        The status should be success
        The stderr should be blank
        The stdout should be blank
        The file ./bin/.protoc-3.7.2.pkg should be exist
        The variable PKG_TEST_VAR should equal "test_value_2"
      End
    End

    Describe "Downgrading a package"
      It "downgrades package to the specified version"
        When call prompt_hooks hermit install protoc-3.7.1
        The status should be success
        The stderr should be blank
        The stdout should be blank
        The file ./bin/.protoc-3.7.2.pkg should not be exist
        The file ./bin/.protoc-3.7.1.pkg should be exist
        The variable PKG_TEST_VAR should equal "=test\"value\""
      End
    End

    Describe "Installing env packages"
      It "is a no-op"
        When call prompt_hooks hermit install
        The status should be success
        The stderr should be blank
        The stdout should be blank
      End
    End

    Describe "deactivating the environment"
      It "removes environment variables and resets the prompt"
        When call deactivate-hermit
        The status should be success
        The stdout should not be blank
        The stderr should be blank
        The variable HERMIT_ENV should be undefined
      End

      It "restores the previous environment variable values"
        When call deactivate-hermit
        The status should be success
        The stdout should not be blank
        The stderr should be blank
        The variable PKG_TEST_VAR should equal "before"
      End
    End
  End

  Describe "switching to another environment"
    Before ". ./bin/activate-hermit"
    It "deactivates the old environment and activates the new one"
      cd ../anotherenv
      hermit init . 2> /dev/null
      When call source ./bin/activate-hermit
      The status should be success
      The stdout should not be blank
      The stderr should be blank
      The variable HERMIT_ENV should equal "$(pwd)"
    End

    It "returns an error if trying to run a binary from the original environments"
      cd ../anotherenv
      . bin/activate-hermit
      When call ../testenv/bin/protoc
      The status should not be success
      The stderr should equal "fatal:hermit: can not execute a Hermit managed binary from a non active environment"
    End
  End

  Describe "uninstalling a package"
    It "removes the symlinks from the environment /bin"
      . bin/activate-hermit

      When call prompt_hooks hermit uninstall protoc
      The status should be success
      The stdout should be blank
      The stderr should be blank
      The file ./bin/protoc should not be exist
      The file ./bin/.protoc-3.7.1.pkg should not be exist
      The variable PKG_TEST_VAR should be undefined
    End
  End

  Describe "installing a channel package"
    It "places the channel symlink to the /bin"
      . bin/activate-hermit
      When call prompt_hooks hermit install protoc@stable
      The status should be success
      The stdout should be blank
      The stderr should be blank
      The file ./bin/protoc should be exist
      The file ./bin/.protoc@stable.pkg should be exist
    End

    It "fails if the channel does not exist"
      . bin/activate-hermit
      When call prompt_hooks hermit install protoc@foobar
      The status should not be success
      The stderr should not be blank
    End
  End

  Describe "removing the hermit binaries during active session"
    . bin/activate-hermit

    It "bootstraps Hermit again correctly"
      rm ${HERMIT_STATE_DIR}/pkg/hermit@canary/hermit
      When call hermit list
      The status should be success
      The stderr should not be blank
      The stdout should not be blank
      The file ${HERMIT_STATE_DIR}/pkg/hermit@canary/hermit should be exist
    End
  End

  Describe "executing hermit in an unactivated environment works"
    It "shows all commands"
      When call ./bin/hermit --help
      The status should be success
      The stdout should include "Install packages."
      The stderr should be blank
    End
  End

  Describe "Interacting with an old project on an empty state"
    clear_state
    cd ../testoldenv
    It "Installs missing packages correctly"
      When call ./bin/protoc
      The status should be success
      The stdout should not be blank
      The stderr should not be blank
    End
  End
End
