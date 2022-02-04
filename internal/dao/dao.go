package dao

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/cashapp/hermit/errors"
)

const (
	usedAtKey          = "usedAt"
	updateCheckedAtKey = "updateCheckedAt"
	eTagKey            = "etag"
	environmentsKey    = "environments"
	timeformat         = time.RFC3339
)

// DAO abstracts away the database access
type DAO struct {
	stateDir string
}

// Package is the package information stored in the DB
type Package struct {
	UsedAt          time.Time
	Etag            string
	UpdateCheckedAt time.Time
}

// Open returns a new DAO at the given state directory
func Open(stateDir string) *DAO {
	return &DAO{stateDir: stateDir}
}

// Dump content of database to w.
func (d *DAO) Dump(w io.Writer) error {
	db, err := d.db()
	if err != nil {
		return errors.WithStack(err)
	}
	tx, err := db.Begin(false)
	if err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(tx.ForEach(func(name []byte, b *bolt.Bucket) error {
		fmt.Fprintf(w, "%s\n", name)
		return dumpBucket(w, 2, b)
	}))
}

// GetPackage returns information of a specific package
func (d *DAO) GetPackage(name string) (*Package, error) {
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

// GetUnusedSince return packages that have not been used since the given time
func (d *DAO) GetUnusedSince(usedAt time.Time) ([]string, error) {
	res := []string{}
	err := d.view(func(tx *bolt.Tx) error {
		return tx.ForEach(func(name []byte, b *bolt.Bucket) error {
			pkg := packageAt(b)
			if pkg.UsedAt.Before(usedAt) {
				res = append(res, string(name))
			}
			return nil
		})
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return res, nil
}

// UpdatePackageWithUsage updates the given package and records its installation directory
func (d *DAO) UpdatePackageWithUsage(binDir string, name string, pkg *Package) error {
	return errors.WithStack(d.update(func(tx *bolt.Tx) error {
		now := time.Now()
		b, err := tx.CreateBucketIfNotExists([]byte(name))
		if err != nil {
			return errors.WithStack(err)
		}
		if err := putPackage(b, pkg); err != nil {
			return errors.WithStack(err)
		}
		ub, err := b.CreateBucketIfNotExists([]byte(environmentsKey))
		if err != nil {
			return errors.WithStack(err)
		}
		return putTime(ub, binDir, now)
	}))
}

// UpdatePackage Updates the update check time, etag, and the used at time for a package
func (d *DAO) UpdatePackage(name string, pkg *Package) error {
	return errors.WithStack(d.update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(name))
		if err != nil {
			return errors.WithStack(err)
		}
		return putPackage(b, pkg)
	}))
}

// PackageRemovedAt Removes a package installation from the DB
func (d *DAO) PackageRemovedAt(name string, binDir string) error {
	return errors.WithStack(d.update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(name))
		if b != nil {
			ub, err := b.CreateBucketIfNotExists([]byte(environmentsKey))
			if err != nil {
				return errors.WithStack(err)
			}
			return ub.Delete([]byte(binDir))
		}
		return nil
	}))
}

// GetKnownUsages returns a list of bin directories where this package has been seen previously
func (d *DAO) GetKnownUsages(name string) ([]string, error) {
	db, err := d.db()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer db.Close()

	res := []string{}
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(name))
		if b != nil {
			ub := b.Bucket([]byte(environmentsKey))
			if ub != nil {
				return ub.ForEach(func(k, _ []byte) error {
					res = append(res, string(k))
					return nil
				})
			}
		}
		return nil
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return res, nil
}

// DeletePackage removes a package from the DB
func (d *DAO) DeletePackage(name string) error {
	db, err := d.db()
	if err != nil {
		return errors.WithStack(err)
	}
	defer db.Close()

	return errors.WithStack(db.Update(func(tx *bolt.Tx) error {
		return tx.DeleteBucket([]byte(name))
	}))
}

func (d *DAO) db() (*bolt.DB, error) {
	return bolt.Open(filepath.Join(d.stateDir, "hermit.bolt.db"), 0600, &bolt.Options{Timeout: 5 * time.Second})
}

func (d *DAO) view(fn func(tx *bolt.Tx) error) error {
	db, err := d.db()
	if err != nil {
		return errors.WithStack(err)
	}
	defer db.Close()

	return errors.WithStack(db.View(fn))
}

func (d *DAO) update(fn func(tx *bolt.Tx) error) error {
	db, err := d.db()
	if err != nil {
		return errors.WithStack(err)
	}
	defer db.Close()

	return errors.WithStack(db.Update(fn))
}

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

func putTime(bucket *bolt.Bucket, name string, value time.Time) error {
	str := value.Format(timeformat)
	return bucket.Put([]byte(name), []byte(str))
}

func putString(bucket *bolt.Bucket, name string, value string) error {
	return bucket.Put([]byte(name), []byte(value))
}

func packageAt(b *bolt.Bucket) *Package {
	if b == nil {
		return nil
	}
	return &Package{
		UsedAt:          timeAt(b, usedAtKey),
		Etag:            stringAt(b, eTagKey),
		UpdateCheckedAt: timeAt(b, updateCheckedAtKey),
	}
}

func putPackage(b *bolt.Bucket, pkg *Package) error {
	if err := putString(b, eTagKey, pkg.Etag); err != nil {
		return err
	}
	if err := putTime(b, updateCheckedAtKey, pkg.UpdateCheckedAt); err != nil {
		return err
	}
	return putTime(b, usedAtKey, pkg.UsedAt)
}

func dumpBucket(w io.Writer, indent int, b *bolt.Bucket) error {
	return b.ForEach(func(k, v []byte) error {
		fmt.Fprintf(w, "%s%s: %s\n", strings.Repeat(" ", indent), k, v)
		if v == nil {
			if err := dumpBucket(w, indent+2, b.Bucket(k)); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	})
}
