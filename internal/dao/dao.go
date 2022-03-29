package dao

import (
	"io"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"

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
	if err := os.MkdirAll(metadataDir, 0700); err != nil && !os.IsExist(err) {
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
		return d.getPackageFromBBolt(pkgRef)
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

// TODO: Remove this BBolt code.

func (d *DAO) db(readonly bool) (*bolt.DB, error) {
	path := filepath.Join(d.stateDir, "hermit.bolt.db")
	db, err := bolt.Open(path, 0600, &bolt.Options{
		Timeout:  5 * time.Second,
		ReadOnly: readonly,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open Hermit state database: %s", path)
	}
	return db, nil
}

func (d *DAO) view(fn func(tx *bolt.Tx) error) error {
	db, err := d.db(true)
	if err != nil {
		return errors.WithStack(err)
	}
	defer db.Close()

	return errors.WithStack(db.View(fn))
}

func (d *DAO) getPackageFromBBolt(name string) (*Package, error) {
	var pkg *Package
	err := d.view(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(name))
		pkg = packageAt(b)
		return nil
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return pkg, nil
}

const (
	updateCheckedAtKey = "updateCheckedAt"
	eTagKey            = "etag"
	timeformat         = time.RFC3339
)

func stringAt(bucket *bolt.Bucket, name string) string {
	if bucket == nil {
		return ""
	}
	bytes := bucket.Get([]byte(name))
	if bytes == nil {
		return ""
	}
	return string(bytes)
}

func timeAt(bucket *bolt.Bucket, name string) time.Time {
	if bucket == nil {
		return time.Time{}
	}
	bytes := bucket.Get([]byte(name))
	if bytes == nil {
		return time.Time{}
	}
	t, err := time.Parse(timeformat, string(bytes))
	if err != nil {
		return time.Time{}
	}
	return t
}

func packageAt(b *bolt.Bucket) *Package {
	if b == nil {
		return nil
	}
	return &Package{
		Etag:            stringAt(b, eTagKey),
		UpdateCheckedAt: timeAt(b, updateCheckedAtKey),
	}
}
