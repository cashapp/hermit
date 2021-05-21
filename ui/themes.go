package ui

var (
	themes = []struct {
		bars  []string
		fill  string
		blank string
	}{
		{[]string{"▏", "▎", "▍", "▌", "▋", "▊", "▉", "█"}, "█", "⣀"},
		{[]string{"▖", "▘", "▝", "▗", "▞", "▚", "▙", "▛", "▜", "▟"}, "█", "⣀"},
		{[]string{"░", "▒", "▓", "█"}, "█", "░"},
		{[]string{"░", "▒", "▓", "█"}, "█", "⣀"},
		{[]string{"▰"}, "▰", "▱"},
		{[]string{"⬤"}, "⬤", "○"},
		{[]string{"◼"}, "◼", "▭"},
		{[]string{"◼", "■"}, "■", "□"},
		{[]string{"⣿"}, "⣿", "⣀"},
	}
	theme = themes[0]
)
