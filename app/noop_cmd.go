package app

type noopCmd struct{}

func (n *noopCmd) Run() error { return nil }
