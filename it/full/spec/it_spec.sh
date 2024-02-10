Describe "Hermit"
  cd testenv

  Describe "Initialisation"
    It "runs successfully"
      When run command hermit init . --idea
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
    It "creates intellij idea plugin configuration in ./.idea/externalDependencies.xml"
      The file ./.idea/externalDependencies.xml should be exist
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

      It "does not overwrite user-overridden variables"
        export PKG_TEST_VAR="modified"
        When call deactivate-hermit
        The status should be success
        The stdout should not be blank
        The stderr should be blank
        The variable PKG_TEST_VAR should equal "modified"
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

    It "can run stubs from other environments"
      cd ../anotherenv
      . bin/activate-hermit
      When call ../testenv/bin/protoc --version
      The status should be success
      The stderr should be blank
      The stdout should not be blank
    End

    It "stub from other environment get environment variables"
      cd ../anotherenv
      . bin/activate-hermit
      When call ../testenv/bin/hermit env
      The status should be success
      The stderr should be blank
      The stdout should include "GOBIN='$(cd ../testenv && pwd)/out/bin'"
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

  Describe "installing a git package from the default branch"
    . bin/activate-hermit
    It "installs from the default branch"
      When call hermit install gitsource@head
      The status should be success
      The stdout should be blank
      The stderr should be blank
      The file ./bin/gitbin should be exist
    End

    It "installs the default version"
      When call ./bin/gitbin
      The status should be success
      The stderr should be blank
      The stdout should include "2.0.0"
      The stdout should include "GNU Make 4.3"
    End
  End

  Describe "installing a git package from a tag"
    . bin/activate-hermit
    It "installs from a tag"
      When call hermit install gitsource-1.0.0
      The status should be success
      The stderr should be blank
      The file ./bin/gitbin should be exist
    End

    It "installs the tagged version"
      When call ./bin/gitbin
      The status should be success
      The stderr should be blank
      The stdout should include "1.0.0"
    End
  End

  Describe "installing a git package from a tagged channel"
    . bin/activate-hermit
    It "installs from a tag"
      When call hermit install gitsource@1
      The status should be success
      The stderr should be blank
      The file ./bin/gitbin should be exist
    End

    It "installs the tagged version"
      When call ./bin/gitbin
      The status should be success
      The stderr should be blank
      The stdout should include "1.0.0"
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

  Describe "Running the add-digests command"
    populate_digest(){
      sum=$(sha256sum ./bin/testbin3.tar.gz | cut -f 1 -d " ")
      hermit manifest add-digests testbin3.hcl
      sum2=$(grep -A 1 "sha256sums" testbin3.hcl | tail -1 | cut -f 2 -d ":" | sed -e 's/",//g')
      [ "$sum" = "$sum2" ]
      grep -q $sum testbin3.hcl
    }
    It "testbin3.hcl add digest"
      cp ../../packages/testbin3.hcl .
      When call populate_digest
      The status should be success
      The stdout should be blank
      The stderr should be blank
    End
  End

  Describe "Check if hermit verifies the digest correctly from sha256sums"
    ensure_digest_check(){
      sum=$(sha256sum ./bin/testbin3.tar.gz | cut -f 1 -d " ")
      hermit manifest add-digests testbin3.hcl
      cp bin/hermit.hcl bin/hermit.hcl.bak
      echo "sources = [\"env:///bin\"]" > bin/hermit.hcl
      badhash="aaaaaaaaaaaaaaaaaaaaaaaaaaaa"
      sed -i -e "s/${sum}/${badhash}/g" testbin3.hcl
      grep -q $badhash testbin3.hcl
      cp testbin3.hcl bin/.

      . ./bin/activate-hermit
      # this should fail because of wrong hash
      rc=0
      # capturing the error because I need to cleanup for other tests.
      hermit install testbin3-1.0.0 || rc=1

      #cleanup
      cp bin/hermit.hcl.bak bin/hermit.hcl
      deactivate-hermit
      # this makes sure that hermit install failed.
      [ $rc = 1 ]
    }
    It "ensure installation fails on bad digest"
    cp ../../packages/testbin3.hcl .
    When call ensure_digest_check
    The status should be success
    The stdout should include "activated"
    The stderr should include " should have been aaaaaaaaaaaaaaaaaaaaaaaaaaaa"
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

  Describe "Runtime dependencies"
    . bin/activate-hermit
    It "do not install to the environment"
      When call hermit install testbin1
      The status should be success
      The stderr should be blank
      The file ./bin/testbin1 should be exist
      The file ./bin/testbin2 should not be exist
    End

    It "allow installing packages with conflicting binaries"
      When call hermit install faketestbin2
      The status should be success
      The stderr should be blank
    End

    It "calls the runtime dependency correctly"
      When call ./bin/testbin1
      The status should be success
      The stdout should equal "Hello from testbin2"
      The stderr should be blank
    End
  End

  Describe "Environments with symbolic links in the path"
    ln -s . ./symlinked
    cd ./symlinked
    . bin/activate-hermit
    It "allows calling binaries in the environment"
      When call ./bin/testbin1
      The status should be success
      The stdout should not be blank
      The stderr should be blank
    End
  End

  Describe "Isolated Runtime dependencies"
    cd ../isolatedenv1
    hermit init .
    source ./bin/activate-hermit
    hermit install gitsource@1
    cp ../../testbins/isolatedenv1 .

    It "can use gitsource@1 from isolatedenv1"
      When call ./bin/gitbin
      The status should be success
      The stdout should include "1.0.0"
      The stderr should be blank
    End

    cd ../isolatedenv2
    hermit init .
    source ./bin/activate-hermit
    hermit install gitsource@head
    cp ../../testbins/isolatedenv2 .

    It "can use gitsource@head from isolatedenv2"
      When call ./bin/gitbin
      The status should be success
      The stdout should include "2.0.0"
      The stderr should be blank
    End

    cd ../testenv
    source ./bin/activate-hermit
    hermit install isolatedenv1
    hermit install isolatedenv2

    It "can call isolatedenv1 binary from testenv"
      When call ./bin/isolatedenv1
      The status should be success
      The stdout should include "Calling gitsource from isolatedenv1"
      The stdout should include "1.0.0"
      The stderr should be blank
    End

    It "can call isolatedenv2 binary from testenv"
      When call ./bin/isolatedenv2
      The status should be success
      The stdout should include "Calling gitsource from isolatedenv2"
      The stdout should include "2.0.0"
      The stderr should be blank
    End
  End
End
