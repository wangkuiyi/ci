package db_test

import (
	"testing"

	"os"

	"github.com/wangkuiyi/ci/db"
)

const testPath = "tmp.test.db"

func TestBuildCreateQuery(t *testing.T) {
	d, err := db.Open(testPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := d.Close()
		if err != nil {
			t.Fatal(err)
		}
	}()
	defer os.Remove(testPath)

	_, err = d.Build(0)
	if err == nil {
		t.FailNow()
	}
	b, err := d.CreateBuild(db.Push, "url", "ref", "sha")
	if err != nil {
		t.Fatal(err)
	}

	bb, err := d.Build(b.ID)
	if err != nil {
		t.Fatal(err)
	}

	if bb != b {
		t.Fatal(b, bb)
	}
}

func TestPendingBuilds(t *testing.T) {
	d, err := db.Open(testPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := d.Close()
		if err != nil {
			t.Fatal(err)
		}
	}()
	defer os.Remove(testPath)

	b, err := d.CreateBuild(db.Push, "url", "ref", "sha")
	if err != nil {
		t.Fatal(err)
	}

	bs, err := d.PendingBuilds()
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 1 || bs[0] != b {
		t.FailNow()
	}

	err = b.SetStatus(db.BuildSuccess)
	if err != nil {
		t.Fatal(err)
	}

	bs, err = d.PendingBuilds()
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 0 {
		t.Fatal(bs)
	}
}

func TestRefBuilds(t *testing.T) {
	d, err := db.Open(testPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := d.Close()
		if err != nil {
			t.Fatal(err)
		}
	}()
	defer os.Remove(testPath)

	b0, err := d.CreateBuild(db.PullRequest, "url", "ref", "sha0")
	if err != nil {
		t.Fatal(err)
	}

	b1, err := d.CreateBuild(db.PullRequest, "url", "ref", "sha1")
	if err != nil {
		t.Fatal(err)
	}

	b2, err := d.CreateBuild(db.PullRequest, "url", "ref", "sha2")
	if err != nil {
		t.Fatal(err)
	}

	b3, err := d.CreateBuild(db.Push, "url", "ref", "sha3")
	if err != nil {
		t.Fatal(err)
	}

	b4, err := d.CreateBuild(db.Push, "url", "ref", "sha4")
	if err != nil {
		t.Fatal(err)
	}

	b5, err := d.CreateBuild(db.Push, "url", "ref", "sha5")
	if err != nil {
		t.Fatal(err)
	}

	bs, err := d.RefBuilds(db.Push, "ref", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 0 {
		t.FailNow()
	}

	bs, err = d.RefBuilds(db.Push, "ref", 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 2 || bs[0] != b5 || bs[1] != b4 {
		t.Fatal(bs)
	}
	bs, err = d.RefBuilds(db.Push, "ref", 0, -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 3 || bs[0] != b5 || bs[1] != b4 || bs[2] != b3 {
		t.Fatal(bs)
	}

	bs, err = d.RefBuilds(db.PullRequest, "ref", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 1 || bs[0] != b2 {
		t.Fatal(bs)
	}
	bs, err = d.RefBuilds(db.PullRequest, "ref", 0, -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 3 || bs[0] != b2 || bs[1] != b1 || bs[2] != b0 {
		t.Fatal(bs)
	}
}

func TestRefs(t *testing.T) {
	d, err := db.Open(testPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := d.Close()
		if err != nil {
			t.Fatal(err)
		}
	}()
	defer os.Remove(testPath)

	refs, err := d.Refs(db.Push)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.FailNow()
	}

	_, err = d.CreateBuild(db.PullRequest, "url", "ref", "sha")
	if err != nil {
		t.Fatal(err)
	}

	refs, err = d.Refs(db.Push)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.FailNow()
	}

	_, err = d.CreateBuild(db.Push, "url", "ref-push", "sha")
	if err != nil {
		t.Fatal(err)
	}

	refs, err = d.Refs(db.PullRequest)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0] != "ref" {
		t.Fatal(refs)
	}

	refs, err = d.Refs(db.Push)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0] != "ref-push" {
		t.Fatal(refs)
	}
}
