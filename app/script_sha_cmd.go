package app

import (
	"fmt"
)

type scriptSHACmd struct{}

func (s *scriptSHACmd) Run(config Config) error {
	for _, sum := range config.SHA256Sums {
		fmt.Println(sum)
	}
	return nil
}
