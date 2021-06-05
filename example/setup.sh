#!/bin/sh

bin=$1

. ./source.sh

kvpath=kv/app/governor/governor/setup

printf 'Get setup secret\n'
setupsecret=$(vault kv get -format json $kvpath | jq -r '.data.data.secret')

printf 'Setup governor\n'
"$bin" setup --secret "$setupsecret"
