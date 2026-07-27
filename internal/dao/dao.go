package dao

import (
	"encoding/json"
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
	if err := os.MkdirAll(metadataDir, 0700); err != nil && !os.IsExist(err) {
		return nil, errors.WithStack(err)
	}
	return &DAO{stateDir: stateDir, metadataDir: metadataDir}, nil
}

// Dump content of database to w.
func (d *DAO) Dump(w io.Writer) error {
	return nil
}

// metadataFile is the on-disk encoding of Package written by UpdatePackage.
//
// UpdateCheckedAt is stored explicitly, rather than inferred from the file's
// mtime (as earlier versions of Hermit did): mtime can't be trusted to mean
// "the moment this etag was written" -- it's disturbed by anything else that
// touches the file (eg. a backup/restore), and differs in precision across
// filesystems.
type metadataFile struct {
	Etag            string    `json:"etag"`
	UpdateCheckedAt time.Time `json:"update_checked_at"`
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
	defer r.Close()
	info, err := r.Stat()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var mf metadataFile
	if err := json.Unmarshal(data, &mf); err != nil {
		// Metadata file written by a Hermit version prior to the
		// introduction of this format: it contains only the raw etag, with
		// no recorded check time. Fall back to the file's mtime, as
		// GetPackage always did previously.
		return &Package{
			Etag:            string(data),
			UpdateCheckedAt: info.ModTime(),
		}, nil
	}
	return &Package{
		Etag:            mf.Etag,
		UpdateCheckedAt: mf.UpdateCheckedAt,
	}, nil
}

// UpdatePackage updates the update check time, etag, and the used at time for a package.
//
// The write is atomic: content is written to a temp file in the same
// directory, then renamed into place. os.WriteFile is not atomic -- it
// truncates the existing file before writing the new content -- so a
// concurrent GetPackage could otherwise observe a torn read (empty or
// partial etag). A torn read here is not merely cosmetic: UpgradeChannel
// treats any etag change, including a corrupted one, as a reason to
// evictPackage (rm -rf) a package tree that another process may be actively
// executing.
func (d *DAO) UpdatePackage(pkgRef string, pkg *Package) error {
	path := d.metadataPath(pkgRef)
	checkedAt := pkg.UpdateCheckedAt
	if checkedAt.IsZero() {
		checkedAt = time.Now()
	}
	data, err := json.Marshal(metadataFile{Etag: pkg.Etag, UpdateCheckedAt: checkedAt})
	if err != nil {
		return errors.WithStack(err)
	}

	tmp, err := os.CreateTemp(d.metadataDir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return errors.WithStack(err)
	}
	tmpPath := tmp.Name()
	// Harmless once the rename below succeeds: nothing left to remove.
	defer os.Remove(tmpPath)

	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil {
		return errors.WithStack(writeErr)
	}
	if closeErr != nil {
		return errors.WithStack(closeErr)
	}
	return errors.WithStack(os.Rename(tmpPath, path))
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
