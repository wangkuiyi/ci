.PHONY: default deps ci_server
export GOPATH:=$(shell pwd)
default: all

deps: 
	go get -d -v ci/...

ci_server: deps
	go install ci/simple_ci

all: ci_server
