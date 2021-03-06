package db

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"

	"fmt"

	"github.com/boltdb/bolt"
	"github.com/topicai/candy"
)

var (
	buildBucket   = []byte("build")
	pendingBucket = []byte("pending")
	statusBucket  = []byte("status")
	outputBucket  = []byte("output")
	shaBucket     = []byte("sha")
	refBucket     = []byte("ref")
)

func validate(start, end int) error {
	if start < 0 {
		return fmt.Errorf("invalid argument start: %d", start)
	}

	if end < -1 {
		return fmt.Errorf("invalid argument end: %d", end)
	} else if end >= 0 && start > end {
		return fmt.Errorf("invalid argument start: %d, end: %d", start, end)
	}

	return nil
}

// DB is the database api for ci system.
type DB struct {
	db *bolt.DB
}

// Open opens a database given path
func Open(path string) (*DB, error) {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, err
	}
	return &DB{db: db}, nil
}

// Close the database.
func (d *DB) Close() error {
	return d.db.Close()
}

// itob returns an 8-byte big endian representation of v
func itob(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

// btoi returns uint64 of 8-byte big endian representation
func btoi(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}

func makeSafeHandler(f func(*bolt.Tx) error) func(*bolt.Tx) error {
	return func(t *bolt.Tx) (err error) {
		defer func() {
			if r := recover(); r != nil {
				switch e := r.(type) {
				case error:
					err = e
				default:
					err = fmt.Errorf("recovered panic: %v", e)
				}
			}
		}()
		return f(t)
	}
}

// CreateBuild creats a build event
func (d *DB) CreateBuild(t BuildType, cloneURL, ref, commitSHA string) (Build, error) {
	build := Build{T: t, CloneURL: cloneURL, Ref: ref, CommitSHA: commitSHA}
	var buildID uint64
	err := d.db.Update(makeSafeHandler(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(buildBucket)
		candy.Must(err)
		buildID, err = b.NextSequence()
		candy.Must(err)
		build.ID = buildID
		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)
		candy.Must(enc.Encode(build))
		candy.Must(b.Put(itob(buildID), buf.Bytes()))
		b, err = tx.CreateBucketIfNotExists(shaBucket)
		candy.Must(err)
		b, err = b.CreateBucketIfNotExists([]byte(build.CommitSHA))
		candy.Must(err)
		commitID, err := b.NextSequence()
		candy.Must(err)
		candy.Must(b.Put(itob(commitID), itob(build.ID)))
		b, err = tx.CreateBucketIfNotExists(refBucket)
		candy.Must(err)
		b, err = b.CreateBucketIfNotExists(itob(uint64(build.T)))
		candy.Must(err)
		b, err = b.CreateBucketIfNotExists([]byte(build.Ref))
		candy.Must(err)
		refID, err := b.NextSequence()
		candy.Must(err)
		candy.Must(b.Put(itob(refID), itob(build.ID)))
		b, err = tx.CreateBucketIfNotExists(pendingBucket)
		candy.Must(err)
		return b.Put(itob(buildID), make([]byte, 0))
	}))
	if err != nil {
		return Build{}, err
	}
	build.db = d.db
	return build, err
}

// Build returns build given build id
func (d *DB) Build(id uint64) (Build, error) {
	var build Build
	err := d.db.View(makeSafeHandler(func(tx *bolt.Tx) error {
		b := tx.Bucket(buildBucket)
		if b == nil {
			return errors.New("buildBucket does not exist")
		}
		v := b.Get(itob(id))
		if v == nil {
			return fmt.Errorf("id %d not exist", id)
		}
		return gob.NewDecoder(bytes.NewReader(v)).Decode(&build)
	}))
	if err != nil {
		return Build{}, err
	}
	build.db = d.db
	return build, nil
}

func (d *DB) idsToBuilds(ids []uint64) ([]Build, error) {
	var bs []Build
	for _, id := range ids {
		b, err := d.Build(id)
		if err != nil {
			return nil, err
		}
		bs = append(bs, b)
	}

	return bs, nil
}

// PendingBuilds returns all pending builds
// pending build is a build that has been created, but not in
// state BuildSuccess, BuildError or BuildFailed
func (d *DB) PendingBuilds() ([]Build, error) {
	ids, err := d.pendingBuilds()
	if err != nil {
		return nil, err
	}

	return d.idsToBuilds(ids)
}

func (d *DB) pendingBuilds() ([]uint64, error) {
	var ids []uint64
	err := d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(pendingBucket)
		if b == nil {
			// no pending bucket
			return nil
		}
		c := b.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			buildID := btoi(k)
			ids = append(ids, buildID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// Refs returns all refs of given build type
func (d *DB) Refs(t BuildType) ([]string, error) {
	var refs []string
	err := d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(refBucket)
		if b == nil {
			return nil
		}
		b = b.Bucket(itob(uint64(t)))
		if b == nil {
			return nil
		}
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			if v == nil {
				refs = append(refs, string(k))
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return refs, nil
}

// RefBuilds returns build given BuildType and ref
// start == 0 means latest one
// if end == -1, will return all data starting from start
func (d *DB) RefBuilds(t BuildType, ref string, start, end int) ([]Build, error) {
	err := validate(start, end)
	if err != nil {
		return nil, err
	}

	if start == end {
		return nil, nil
	}

	ids, err := d.refBuilds(t, ref, start, end)
	if err != nil {
		return nil, err
	}

	return d.idsToBuilds(ids)
}

func (d *DB) refBuilds(t BuildType, ref string, start, end int) ([]uint64, error) {
	var ids []uint64
	diff := end - start
	err := d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(refBucket)
		if b == nil {
			return nil
		}
		b = b.Bucket(itob(uint64(t)))
		if b == nil {
			return nil
		}
		b = b.Bucket([]byte(ref))
		if b == nil {
			return nil
		}
		c := b.Cursor()
		for k, v := c.Last(); (end == -1 || len(ids) < diff) && k != nil; k, v = c.Prev() {
			ids = append(ids, btoi(v))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// SHABuilds returns all build given commit SHA
func (d *DB) SHABuilds(sha string) ([]Build, error) {
	ids, err := d.shaBuilds(sha)
	if err != nil {
		return nil, err
	}

	return d.idsToBuilds(ids)
}

func (d *DB) shaBuilds(sha string) ([]uint64, error) {
	var ids []uint64
	err := d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(shaBucket)
		if b == nil {
			// no pending bucket
			return nil
		}
		b = b.Bucket([]byte(sha))
		if b == nil {
			return nil
		}

		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			ids = append(ids, btoi(v))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}
