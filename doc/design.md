# Write and Host Our Own CI for Github

Yi Wang (yiwang01@baidu.com)

## Host Our Own CI

It is not trivial to run all CI tests on the cloud, say
travis-ci.org. For example, travis-ci.org doesn’t rent GPU VMs, so it
cannot run GPU related code. Another example is to set up a VM cluster
to test the whole installation and upgrading procedure of Kubernetes,
as we do [here](https://github.com/k8sp/sextant/tree/master/vm-cluster).

## Github Integration

As long as our CI runs as a server, Github can notify our CI about
user behaviors as events like git push. Then our CI can checkout the
recently pushed source code, test it, and respond with the status to
Github by calling Github RESTful API.

## Host CI Locally

With the help from ngrok, a tool that tunnels our local server to a
public server, we can run our CI anywhere, like on our notepad in corp
network, and let Github access it from a public URL suffixed with
`ngrok.com`.

## Write Our Own CI

Instead to set up Jenkins, I prefer to write our own. It is super
cheap to write a CI system that works with Github in Go. And we need
the capability of customization -- we might want to set up a VM
cluster, install Kubernetes onto it, and start a distributed Paddle
training job on top of that.  We might even want to map GPUs on the CI
server to each of these VMs.

## How to Write a CI

This [post](https://developer.github.com/guides/building-a-ci-server/)
from Github shows how to write a CI server. For each push event, this
server includes three parts:

1. Get notification from Github about push events via Webhook.
1. Set PR/commit status to “pending”
1. Run CI
1. Set final status to either “failed” or “success”.

The Webhook notification part requires that we set a Wehook for the
repo. With ngrok we don’t have to deploy our CI onto a VPS with public
IP, but could run it on our local computer.

The setting status part requires calling Github Web API with a
token. The set/create status API is described in this Github
[document](https://developer.github.com/v3/repos/statuses/#create-a-status). Above
post shows how to call this API via
[Octokit.rb](https://github.com/octokit/octokit.rb/blob/master/lib/octokit/client/statuses.rb#L41).
We could also call the
[Octokit.go](https://github.com/octokit/go-octokit/blob/master/octokit/statuses.go#L42)
API.
