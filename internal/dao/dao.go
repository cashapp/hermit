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
	stateDir string
}

// Package is the package information stored in the DB
type Package struct {
	Etag            string
	UpdateCheckedAt time.Time
}

// Open returns a new DAO at the given state directory
func Open(stateDir string) (*DAO, error) {
	stateDir = filepath.Join(stateDir, "metadata")
	if err := os.Mkdir(stateDir, 0700); err != nil && !os.IsExist(err) {
		return nil, errors.WithStack(err)
	}
	return &DAO{stateDir: stateDir}, nil
}

// Dump content of database to w.
func (d *DAO) Dump(w io.Writer) error {
	return nil
}

// GetPackage returns information for a specific package.
func (d *DAO) GetPackage(pkgRef string) (*Package, error) {
	pkg := Package{}
	if updatedAt, err := d.getUpdateCheckedAt(pkgRef); err != nil && !os.IsNotExist(err) {
		return nil, errors.WithStack(err)
	} else { // nolint
		pkg.UpdateCheckedAt = updatedAt
	}
	if etag, err := d.getETag(pkgRef); err != nil && !os.IsNotExist(err) {
		return nil, errors.WithStack(err)
	} else { // nolint
		pkg.Etag = etag
	}
	return &pkg, nil
}

// UpdatePackage Updates the update check time, etag, and the used at time for a package
func (d *DAO) UpdatePackage(pkgRef string, pkg *Package) error {
	if err := d.updateCheckedAt(pkgRef); err != nil {
		return errors.WithStack(err)
	}
	if err := d.writeETag(pkgRef, pkg.Etag); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// DeletePackage removes a package from the DB
func (d *DAO) DeletePackage(pkgRef string) error {
	if err := os.Remove(d.metadataPath(pkgRef, "etag")); err != nil {
		return errors.WithStack(err)
	}
	if err := os.Remove(d.metadataPath(pkgRef, "updated")); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (d *DAO) metadataPath(pkgRef, key string) string {
	return filepath.Join(d.stateDir, pkgRef+"."+key)
}

func (d *DAO) getETag(pkgRef string) (string, error) {
	data, err := os.ReadFile(d.metadataPath(pkgRef, "etag"))
	return string(data), errors.WithStack(err)
}

func (d *DAO) writeETag(pkgRef, etag string) error {
	return errors.WithStack(os.WriteFile(d.metadataPath(pkgRef, "etag"), []byte(etag), 0600))
}

func (d *DAO) getUpdateCheckedAt(pkgRef string) (time.Time, error) {
	info, err := os.Stat(d.metadataPath(pkgRef, "updated"))
	if err != nil {
		return time.Time{}, errors.WithStack(err)
	}
	return info.ModTime(), nil
}

func (d *DAO) updateCheckedAt(pkgRef string) error {
	f, err := os.OpenFile(d.metadataPath(pkgRef, "updated"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return errors.WithStack(err)
	}
	return f.Close()
}
