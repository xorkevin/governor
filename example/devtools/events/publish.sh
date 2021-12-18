#!/bin/sh

set -e

ROOT=${0%/*}

subject=$1
payload=$2

if [ -z "$subject" ]; then
  printf "Must provide subject\n"
  exit 1
fi

if [ -z "$payload" ]; then
  printf "Must provide payload\n"
  exit 1
fi

curl --user "system:secret" --request POST "http://localhost:8080/api/events/publish?subject=$subject" --data-raw "$payload"
