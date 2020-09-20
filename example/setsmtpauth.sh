#!/usr/bin/env bash

. ./source.sh

kvpath=kv/external/smtp

read -ep 'username: ' username
read -sp 'password: ' password; printf '\n'

vault kv put $kvpath "username=$username" "password=$password"
