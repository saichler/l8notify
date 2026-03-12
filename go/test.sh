#!/usr/bin/env bash

# Fail on errors
set -e

# clean up
rm -rf go.sum
rm -rf go.mod
rm -rf vendor

# fetch dependencies
go mod init
GOPROXY=direct GOPRIVATE=github.com go mod tidy
go mod vendor

echo "About to run tests"
read -n 1 -s -r -p "Press any key to continue..."

# Run unit tests with coverage
go test -v -coverpkg=./channel/...,./template/...,./throttle/...,./escalation/... -coverprofile=cover.html ./... --failfast

# Open the coverage report in a browser
go tool cover -html=cover.html
