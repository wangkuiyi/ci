package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/topicai/candy"
)

func TestCmd(t *testing.T) {
	assert.NotPanics(t, func() { cmd(nil, "ls", "/") })
	assert.Panics(t, func() { cmd(nil, "something-not-exists") })
}

func TestCmdWithEnv(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "")
	tmpfile := path.Join(tmpdir, "TestRunWithEnv")

	cmd(map[string]string{"GOPATH": "/tmp"},
		"awk",
		fmt.Sprintf("BEGIN{print ENVIRON[\"GOPATH\"] > \"%s\";}", tmpfile))

	b, _ := ioutil.ReadFile(tmpfile)
	assert.Equal(t, "/tmp\n", string(b))
}

func TestCI(t *testing.T) {
	db, e := sql.Open("mysql", fmt.Sprintf("root:@/ci_test"))
	candy.Must(e)
	defer func() { candy.Must(db.Close()) }()

	insert := makeInserter(db)

	ci(&PushEvent{
		After: "d07ac266d969affd4b6f016c1cbbe999f193a567",
		Repository: Repository{
			URL: "https://github.com/wangkuiyi/ci_test/",
		}}, insert)
}
