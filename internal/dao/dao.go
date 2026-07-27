package dao

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cashapp/hermit/errors"
	"github.com/cashapp/hermit/util"
)

// staleScratchAge is how old a leftover ".tmp-*" file must be before Open
// considers it abandoned rather than an in-flight write from another
// process.
const staleScratchAge = 24 * time.Hour

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
	sweepStaleScratchFiles(metadataDir)
	return &DAO{stateDir: stateDir, metadataDir: metadataDir}, nil
}

// sweepStaleScratchFiles removes leftover ".tmp-*" files from
// util.AtomicWriteFile calls that were interrupted by a killed process (eg.
// SIGKILL, which the writer's deferred os.Remove cannot run for). Best
// effort: errors are ignored, and a generous age threshold avoids racing a
// concurrent, genuinely in-flight write from another Hermit process.
func sweepStaleScratchFiles(metadataDir string) {
	entries, err := os.ReadDir(metadataDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !strings.Contains(entry.Name(), ".tmp-") {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < staleScratchAge {
			continue
		}
		_ = os.Remove(filepath.Join(metadataDir, entry.Name()))
	}
}

// Dump content of database to w.
func (d *DAO) Dump(w io.Writer) error {
	return nil
}

// GetPackage returns information for a specific package.
//
// The etag is stored as the raw, unencoded file content at metadataPath: this
// is the exact on-disk format every Hermit version has ever written, so a
// mixed-version fleet sharing a state directory can always read and write it
// identically. UpdateCheckedAt is stored separately, in the sidecar file at
// checkedAtPath, because mtime can't be trusted to mean "the moment this etag
// was written" -- it's disturbed by anything else that touches the file (eg.
// a backup/restore), and differs in precision across filesystems. An older
// Hermit version, or a first-ever check, has no such sidecar: fall back to
// the etag file's mtime in that case, as GetPackage always did previously.
func (d *DAO) GetPackage(pkgRef string) (*Package, error) {
	etag, err := os.ReadFile(d.metadataPath(pkgRef))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	checkedAt, err := d.readCheckedAt(pkgRef)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if checkedAt.IsZero() {
		info, err := os.Stat(d.metadataPath(pkgRef))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		checkedAt = info.ModTime()
	}
	return &Package{
		Etag:            string(etag),
		UpdateCheckedAt: checkedAt,
	}, nil
}

func (d *DAO) readCheckedAt(pkgRef string) (time.Time, error) {
	data, err := os.ReadFile(d.checkedAtPath(pkgRef))
	if os.IsNotExist(err) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, errors.WithStack(err)
	}
	checkedAt, err := time.Parse(time.RFC3339Nano, string(data))
	if err != nil {
		// A torn read of the sidecar (or one written by an incompatible
		// future version) is not fatal: fall back to mtime rather than
		// failing the whole lookup.
		return time.Time{}, nil //nolint:nilerr
	}
	return checkedAt, nil
}

// UpdatePackage updates the update check time and etag for a package.
//
// Both files are written atomically: content is written to a temp file in
// the same directory, then renamed into place. os.WriteFile is not atomic --
// it truncates the existing file before writing the new content -- so a
// concurrent GetPackage could otherwise observe a torn read (empty or
// partial etag). A torn read here is not merely cosmetic: UpgradeChannel
// treats any etag change, including a corrupted one, as a reason to
// evictPackage (rm -rf) a package tree that another process may be actively
// executing.
//
// The etag is written first: if the process dies between the two writes, a
// concurrent GetPackage falls back to the etag file's mtime for
// UpdateCheckedAt (see above), which is the same degraded-but-safe behaviour
// as running against an older Hermit version that never writes the sidecar
// at all.
func (d *DAO) UpdatePackage(pkgRef string, pkg *Package) error {
	checkedAt := pkg.UpdateCheckedAt
	if checkedAt.IsZero() {
		checkedAt = time.Now()
	}
	if err := util.AtomicWriteFile(d.metadataPath(pkgRef), []byte(pkg.Etag), 0600); err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(util.AtomicWriteFile(d.checkedAtPath(pkgRef), []byte(checkedAt.Format(time.RFC3339Nano)), 0600))
}

// DeletePackage removes a package from the DB
func (d *DAO) DeletePackage(pkgRef string) error {
	if err := os.Remove(d.metadataPath(pkgRef)); err != nil {
		return errors.WithStack(err)
	}
	// The checked-at sidecar may not exist (eg. written by an older Hermit
	// version); that's not an error.
	if err := os.Remove(d.checkedAtPath(pkgRef)); err != nil && !os.IsNotExist(err) {
		return errors.WithStack(err)
	}
	return nil
}

func (d *DAO) metadataPath(pkgRef string) string {
	return filepath.Join(d.metadataDir, pkgRef+".etag")
}

func (d *DAO) checkedAtPath(pkgRef string) string {
	return filepath.Join(d.metadataDir, pkgRef+".checked")
}
