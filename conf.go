// The config parsers. Parse the command line option --config, and read the configuration file.
package main

// HTTPOption used for HTTP Server and Github Webhook
type HTTPOption struct {
	Addr   string // http address
	CIUri  string // the uri of Github WebHook
	Secret string // Github WebHook secret

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
	// TODO(helin): at least everything other than scripts should be in config yaml (transparent to user)
	return &Options{
		HTTP: HTTPOption{
			Addr:        ":8000",
			CIUri:       "/ci/",
			Secret:      "",
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
cd {{.BuildPath}}
git clone --depth 1 {{.CloneURL}} repo
cd repo
git fetch origin {{.Ref}}
git checkout -qf {{.Head}}
`,
			ExecuteTpl: `
set +x
if [ -f {{.Filename}} ]; then
	source {{.Filename}}
else
	echo "{{.Filename}} not found, it seems the ci script is not configured."
fi
`,
			CleanTpl: `#!/bin/bash
rm -rf {{.BuildPath}}/*
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
