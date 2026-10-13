#!/bin/sh
# Run every analyzer on application code; upstream pointer arithmetic has one exception.
set -eu
packages=''
driver=''
for package in $(go list ./...); do
 case "$package" in
  */internal/thirdparty/sqlite) driver="$package" ;;
  *) packages="$packages $package" ;;
 esac
done
# Go import paths contain no shell whitespace.
# shellcheck disable=SC2086
go vet $packages
if [ -n "$driver" ]; then go vet -unsafeptr=false "$driver"; fi
