package db_test

import (
	"testing"

	"os"

	"github.com/wangkuiyi/ci/db"
)

func TestBuild(t *testing.T) {
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

	if b.CloneURL != "url" || b.Ref != "ref" || b.CommitSHA != "sha" {
		t.FailNow()
	}

	l, err := b.Output(0, -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(l) != 0 {
		t.FailNow()
	}

	line0 := db.OutputLine{T: db.Stdout, Str: "stdout"}
	line1 := db.OutputLine{T: db.Stderr, Str: "stderr"}
	line2 := db.OutputLine{T: db.Info, Str: "info"}
	line3 := db.OutputLine{T: db.Error, Str: "error"}

	b.AppendOutput(line0)
	b.AppendOutput(line1)
	b.AppendOutput(line2)
	b.AppendOutput(line3)

	l, err = b.Output(0, -1)
	if err != nil {
		t.Fatal(err)
	}

	l, err = b.Output(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(l) != 1 || l[0] != line1 {
		t.Fatal(l)
	}

	l, err = b.Output(0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(l) != 4 || l[0] != line0 || l[1] != line1 || l[2] != line2 || l[3] != line3 {
		t.Fatal(l)
	}

	_, err = b.Status()
	if err == nil {
		// should have error since status of b is not set yet
		t.FailNow()
	}

	err = b.SetStatus(db.BuildRunning)
	if err != nil {
		t.Fatal(err)
	}

	s, err := b.Status()
	if err != nil {
		t.Fatal(err)
	}
	if s != db.BuildRunning {
		t.FailNow()
	}
}
