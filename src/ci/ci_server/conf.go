// The config parsers. Parse the command line option --config, and read the configuration file.
package ci_server

import (
	"flag"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
)

// Option for HTTP Server and Github Webhook
type HttpOption struct {
	Addr   string // http address
	CIUri  string // the uri of Github WebHook
	Secret string // Github WebHook secret

	StatusUri string // the status uri for each build
	Hostname  string // the http host.
}

// Option for github API option
type GithubAPIOption struct {
	Token       string  // github personal token.
	Owner       string  // Repository Owner
	Name        string  // Repository Name
	Description string  // Description for CI System. Such `CI System for mac os GPU`
}


// Option for postgres database
type DatabaseOption struct {
	User         string
	Password     string
	DatabaseName string
}

// Option for builder
type BuildOption struct {
	Concurrent int	// how many build scripts can be performed in parallel.
			// The builds are executed in different directory
	Dir        string  // Base directory for builder. Each go routine Dir will be ${Dir}/id
	Env        map[string]string  // The build environment can be anything. Such as OS=osx OS_VERSION=10.11
	Filename   string  // The ci script filename. default is `./ci.sh`

	BootstrapTpl      string  // bootstrap template, used for setting env
	PushEventCloneTpl string  // push event clone template. used for clone push event
	ExecuteTpl        string  // execution template, used for executing ci.sh
	CleanTpl          string  // clean files.
}

type Options struct {
	// Version, currently version field is not used, but set this field for future capacity.
	Version int

	// HTTP
	HTTP HttpOption

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
		HTTP: HttpOption{
			Addr:      ":8000",
			CIUri:     "/ci/",
			Secret:    "",
			StatusUri: "/status/",
			Hostname:  "",
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
git clone --branch={{.BranchName}} --depth=50 {{.CloneUrl}} repo
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

// Parse arguments.
func ParseArgs() (opts *Options) {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, usage, os.Args[0])
		flag.PrintDefaults()
	}

	fn := flag.String("config", "ci.yaml", "Configuration File")
	flag.Parse()
	opts = newOptions()
	f, err := os.Open(*fn)
	CheckNoErr(err)
	content, err := ioutil.ReadAll(f)
	CheckNoErr(err)
	err = yaml.Unmarshal(content, opts)
	CheckNoErr(err)
	return
}
