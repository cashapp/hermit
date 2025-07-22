env = {
  "BAR": "waz",
  "ESCAPED": "$${DONT_EXPAND_ME}"
}
sources = ["env:///packages"]
