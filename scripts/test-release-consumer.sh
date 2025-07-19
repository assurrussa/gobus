#!/bin/sh

set -eu

version=${1:-}
if [ -z "$version" ]; then
	echo "usage: $0 <published-version>" >&2
	exit 2
fi

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
consumer_tmp=$(mktemp -d)

cleanup() {
	rm -rf "$consumer_tmp"
}
trap cleanup EXIT

cp "$script_dir/../testdata/consumer/consumer_test.go" "$consumer_tmp/consumer_test.go"
cd "$consumer_tmp"

go mod init example.com/gobus-release-consumer
go mod edit -go=1.27.0
go get "github.com/assurrussa/gobus@$version"
go test ./...
