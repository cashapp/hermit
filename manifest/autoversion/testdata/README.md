# Auto-versioning test data

There are three possible inputs for each test case in this directory.

1. `<test>.input.hcl` - the input manifest with auto-versioning configured
2. `<test>.expected.hcl` - the expected updated manifest content after auto-versioning completes
3. (optional) `<test>.http` - if present, the returned content of any HTTP request sent to the auto-version HTTP client.