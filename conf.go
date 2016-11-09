// The config parsers. Parse the command line option --config, and read the configuration file.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"gopkg.in/yaml.v2"
)

// HTTPOption used for HTTP Server and Github Webhook
type HTTPOption struct {
	Addr   string // http address
	CIUri  string // the uri of Github WebHook
	Secret string // Github WebHook secret

	StatusURI   string // the status uri for each build
	Hostname    string // the http host.
	TemplateDir string // the directory for html template
	StaticDir   string // the directory for static file
}

// GithubAPIOption for github API option
type GithubAPIOption struct {
	Token       string // github personal token.
	Owner       string // Repository Owner
	Name        string // Repository Name
	Description string // Description for CI System. Such `CI System for mac os GPU`
}

// DatabaseOption for postgres database
type DatabaseOption struct {
	User         string
	Password     string
	DatabaseName string
}

// BuildOption for builder
type BuildOption struct {
	Concurrent int // how many build scripts can be performed in parallel.
	// The builds are executed in different directory
	Dir      string            // Base directory for builder. Each go routine Dir will be ${Dir}/id
	Env      map[string]string // The build environment can be anything. Such as OS=osx OS_VERSION=10.11
	Filename string            // The ci script filename. default is `./ci.sh`

	BootstrapTpl      string // bootstrap template, used for setting env
	PushEventCloneTpl string // push event clone template. used for clone push event
	ExecuteTpl        string // execution template, used for executing ci.sh
	CleanTpl          string // clean files.
}

// Options in yaml configration file.
type Options struct {
	// Version, currently version field is not used, but set this field for future capacity.
	Version int

	// HTTP
	HTTP HTTPOption

	// Database
	Database DatabaseOption

	// Build Config
	Build BuildOption

	// Github API Option
	Github GithubAPIOption
}

// Create new options with default values.
func newOptions() *Options {
	return &Options{
		HTTP: HTTPOption{
			Addr:        ":8000",
			CIUri:       "/ci/",
			Secret:      "",
			StatusURI:   "/status/",
			Hostname:    "",
			TemplateDir: "./templates/",
		},
		Database: DatabaseOption{
			User:         "DEBUG",
			Password:     "DEBUG",
			DatabaseName: "ci",
		},
		Build: BuildOption{
			Concurrent: 1,
			Dir:        "./build",
			Env:        make(map[string]string),
			BootstrapTpl: `#!/bin/bash
echo "Setting Environments"
set -x
{{range $envKey, $envVal := .Env}}
export {{$envKey}}="{{$envVal}}"
{{end}}
set +x
set -e
`,
			PushEventCloneTpl: `
set -x
git clone --branch={{.BranchName}} --depth=50 {{.CloneURL}} repo
cd repo
git checkout -qf {{.Head}}
`,
			ExecuteTpl: `
set +x
if [ -f {{.Filename}} ]; then
	{{.Filename}}
else
	echo "{{.Filename}} not found, it seems the ci script is not configured."
fi
`,
			CleanTpl: `#!/bin/bash
rm -rf repo
`,
			Filename: "./ci.sh",
		},
		Github: GithubAPIOption{
			Token:       "",
			Description: "",
		},
	}
}

const usage string = `Usage %s [OPTIONS]
Options:
`

// ParseArgs is used for parsing command line argument, and read the configuration file, then return the Options
func ParseArgs() (opts *Options) {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, usage, os.Args[0])
		flag.PrintDefaults()
	}

	fn := flag.String("config", "ci.yaml", "Configuration File")
	flag.Parse()
	opts = newOptions()
	f, err := os.Open(*fn)
	checkNoErr(err)
	content, err := ioutil.ReadAll(f)
	checkNoErr(err)
	err = yaml.Unmarshal(content, opts)
	checkNoErr(err)
	return
}
