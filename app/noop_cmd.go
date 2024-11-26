package app

type noopCmd struct {
	Discard []string `arg:"" optional:""`
}

func (n *noopCmd) Run() error { return nil }
