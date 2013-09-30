#!/usr/bin/env bash

export PATH="$PATH:`echo $GOPATH|cut -f1 -d:`/bin"

if [ ! -z $COVERALLS_TOKEN ]; then
	goveralls -package=./... $COVERALLS_TOKEN
else 
	go test -v ./...
fi
