package main

import (
	"fmt"
	"io/ioutil"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
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
