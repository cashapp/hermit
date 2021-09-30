package app

import "fmt"

type dumpUserConfigSchema struct{}

func (dumpUserConfigSchema) Run() error {
	fmt.Print(userConfigSchema)
	return nil
}
