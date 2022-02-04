package app

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/ui"
)

type validateEnvCmd struct {
	Env string `arg:"" type:"existingdir" help:"Path to environment root."`
}

func (v *validateEnvCmd) Run(l *ui.UI, config Config) error {
next:
	for _, path := range []string{"bin/activate-hermit", "bin/hermit"} {
		path = filepath.Join(v.Env, path)
		hasher := sha256.New()
		r, err := os.Open(path)
		if os.IsNotExist(err) {
			return errors.Errorf("%s is missing, not a Hermit environment?", path)
		} else if err != nil {
			return errors.WithStack(err)
		}
		_, err = io.Copy(hasher, r)
		if err != nil {
			return errors.WithStack(err)
		}
		hash := hex.EncodeToString(hasher.Sum(nil))
		l.Debugf("%s %s\n", hash, path)
		for _, candidate := range config.SHA256Sums {
			if hash == candidate {
				l.Infof("%s validated as %s", path, hash)
				continue next
			}
		}
		return errors.Errorf("%s has an unknown SHA256 signature (%s); verify that you trust this environment and run 'hermit init %s'", path, hash, v.Env)
	}
	l.Infof("%s ok", v.Env)
	return nil
}
