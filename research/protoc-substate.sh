#!/bin/bash

#go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28.1

protoc --go_out=. substate.proto
