#!/bin/bash

cd $(dirname $0)
for d in basic buffalo gin goji martini negroni nethttp
do
	echo Building $d
	pushd $d >/dev/null
	go get || exit $?
	go build || exit $?
	popd >/dev/null
done
