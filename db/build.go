package db

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"time"

	"github.com/boltdb/bolt"
	"github.com/topicai/candy"
)

// LineType is the type of a line in output
type LineType int

// output line types
const (
	Stdout LineType = iota
	Stderr
	Info
	Error
)

// OutputLine is a line in output
type OutputLine struct {
	T    LineType
	Time time.Time
	Str  string
}

// BuildType is the type of build
type BuildType uint64

const (
	// PullRequest means build is triggered from pull request event
	PullRequest BuildType = iota
	// Push means build is triggered from push event
	Push
)

// BuildStatus in database
type BuildStatus string

// build status
const (
	BuildQueued  = "queued"
	BuildRunning = "running"
	BuildSuccess = "success"
	// BuildError means there is error during build caused by build system
	BuildError = "error"
	// BuildFailed means there is error during build caused by build script
	BuildFailed = "failed"
)

// Build represents a build event in database
// the coresponding value of public field in database will never change
type Build struct {
	db *bolt.DB

	T         BuildType
	Ref       string
	CloneURL  string
	CommitSHA string
	ID        uint64
}

// SetStatus sets build status
func (b *Build) SetStatus(s BuildStatus) error {
	err := b.db.Update(makeSafeHandler(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(statusBucket)
		candy.Must(err)
		candy.Must(bucket.Put(itob(b.ID), []byte(s)))
		if s == BuildFailed || s == BuildSuccess || s == BuildError {
			// remove from pending
			bucket = tx.Bucket(pendingBucket)
			if bucket == nil {
				return nil
			}

			err = bucket.Delete(itob(b.ID))
			candy.Must(err)
		}
		return nil
	}))
	return err
}

// Status returns build status
func (b *Build) Status() (BuildStatus, error) {
	var stat BuildStatus
	err := b.db.View(makeSafeHandler(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(statusBucket)
		if bucket == nil {
			return errors.New("statusBucket not exist")
		}
		v := bucket.Get(itob(b.ID))
		if v == nil {
			return fmt.Errorf("build status not created for build id %d", b.ID)
		}
		stat = BuildStatus(v)
		return nil
	}))
	if err != nil {
		return "", err
	}
	return stat, nil
}

// AppendOutput append output for a build
func (b *Build) AppendOutput(o OutputLine) error {
	if o.Str == "" {
		return nil
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(o)
	if err != nil {
		return err
	}

	err = b.db.Update(makeSafeHandler(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(outputBucket)
		candy.Must(err)
		bucket, err = bucket.CreateBucketIfNotExists(itob(b.ID))
		candy.Must(err)
		id, err := bucket.NextSequence()
		candy.Must(err)
		return bucket.Put(itob(id), buf.Bytes())
	}))
	return err
}

// Output returns output of a build in a range
// if end == -1, will return all data starting from start
func (b *Build) Output(start, end int) ([]OutputLine, error) {
	err := validate(start, end)
	if err != nil {
		return nil, err
	}

	if start == end {
		return nil, nil
	}

	diff := -1
	if end >= 0 {
		diff = end - start
	}

	// db sequence starts from 1, increment start by 1
	start++

	var out []OutputLine
	err = b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(outputBucket)
		if bucket == nil {
			// treat as no output
			return nil
		}
		bucket = bucket.Bucket(itob(b.ID))
		if bucket == nil {
			// treat as no output
			return nil
		}
		c := bucket.Cursor()
		key := itob(uint64(start))
		k, v := c.Seek(key)
		if !bytes.Equal(key, k) {
			// key not exist, start is too big
			return nil
		}

		count := 0
		for ; (diff == -1 || count < diff) && k != nil; k, v = c.Next() {
			count++
			var o OutputLine
			candy.Must(gob.NewDecoder(bytes.NewReader(v)).Decode(&o))
			out = append(out, o)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
