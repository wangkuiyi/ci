# use wheezy because it contains lots of stuff that typical build script needs. E.g., git
FROM golang:1.7.4-wheezy

RUN apt-get update && apt-get install -y ca-certificates
# install docker client, so ci can do docker build, and start built docker to run test
RUN cd / && wget https://get.docker.com/builds/Linux/x86_64/docker-1.11.0.tgz && tar -xvf docker-1.11.0.tgz && mv /docker/docker /usr/bin/ && rm -rf /docker
RUN mkdir /data
ADD . /gopath/src/github.com/wangkuiyi/ci
RUN cd /gopath/src/github.com/wangkuiyi/ci/ && GOPATH=/gopath go get ./... && GOPATH=/gopath CGO_ENABLED=0 go build && cp ci / && rm -rf /gopath
ADD templates /templates
EXPOSE 8000
ENTRYPOINT ["/ci"]
