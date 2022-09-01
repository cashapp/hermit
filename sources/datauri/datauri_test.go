package datauri

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test(t *testing.T) {
	for _, expected := range [][]byte{
		[]byte("{}"),
		[]byte("{\"foo\":\"bar\"}"),
		[]byte("[\n  1,\n  2\n]\n"),
	} {
		t.Run(string(expected), func(t *testing.T) {
			encoded := Encode(expected)
			fmt.Println(encoded)
			actual, err := Decode(encoded)
			require.NoError(t, err)
			require.Equal(t, expected, actual)
		})
	}
}
