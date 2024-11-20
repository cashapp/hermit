package dao

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/cashapp/hermit/errors"
)

// DAO abstracts away the database access
type DAO struct {
	stateDir    string
	metadataDir string
}

// Package is the package information stored in the DB
type Package struct {
	Etag            string
	UpdateCheckedAt time.Time
}

// Open returns a new DAO at the given state directory
func Open(stateDir string) (*DAO, error) {
	metadataDir := filepath.Join(stateDir, "metadata")
	if err := os.MkdirAll(metadataDir, 0777); err != nil && !os.IsExist(err) {
		return nil, errors.WithStack(err)
	}
	return &DAO{stateDir: stateDir, metadataDir: metadataDir}, nil
}

// Dump content of database to w.
func (d *DAO) Dump(w io.Writer) error {
	return nil
}

// GetPackage returns information for a specific package.
func (d *DAO) GetPackage(pkgRef string) (*Package, error) {
	r, err := os.Open(d.metadataPath(pkgRef))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer r.Close() // nolint
	info, err := r.Stat()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	etag, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &Package{
		Etag:            string(etag),
		UpdateCheckedAt: info.ModTime(),
	}, nil
}

// UpdatePackage Updates the update check time, etag, and the used at time for a package
func (d *DAO) UpdatePackage(pkgRef string, pkg *Package) error {
	return errors.WithStack(os.WriteFile(d.metadataPath(pkgRef), []byte(pkg.Etag), 0600))
}

// DeletePackage removes a package from the DB
func (d *DAO) DeletePackage(pkgRef string) error {
	if err := os.Remove(d.metadataPath(pkgRef)); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (d *DAO) metadataPath(pkgRef string) string {
	return filepath.Join(d.metadataDir, pkgRef+".etag")
}
