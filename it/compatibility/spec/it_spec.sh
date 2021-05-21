Describe "Hermit Backwards Compatibility"
  cd testenv

  Describe "Activating the environment"
      It "sets the environment correctly"
        When call source ./bin/activate-hermit
        The status should be success
        The stdout should not be blank
        The stderr should be blank
        The variable HERMIT_ENV should equal "$(pwd)"
      End
  End

  Describe "Interacting with the active environment"
    Before ". ./bin/activate-hermit"

    Describe "Installing a new package"
      It "places the symlinks in the environment /bin"
        When call hermit install protoc-3.7.1
        The status should be success
        The stderr should be blank
        The file ./bin/protoc should be exist
        The file ./bin/.protoc-3.7.1.pkg should be exist
      End
    End
  End
End