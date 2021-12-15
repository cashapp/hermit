package app

import (
	"github.com/pkg/errors"

	"github.com/cashapp/hermit/github"
	"github.com/cashapp/hermit/manifest"
	"github.com/cashapp/hermit/ui"
)

type autoVersionCmd struct {
	Manifest []string `arg:"" type:"existingfile" required:"" help:"Manifests to upgrade." predictor:"hclfile"`
}

func (s *autoVersionCmd) Run(l *ui.UI, client *github.Client) error {
	for _, path := range s.Manifest {
		l.Debugf("Auto-versioning %s", path)
		version, err := manifest.AutoVersion(client, path)
		if err != nil {
			return errors.WithStack(err)
		}
		if version != "" {
			l.Infof("Auto-versioned %s to %s", path, version)
		}
	}
	return nil
}
