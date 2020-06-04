#!/usr/bin/env bash

export VAULT_ADDR=http://127.0.0.1:8200/

kvpath=kv/external/smtp

read -ep 'username: ' username
read -sp 'password: ' password; printf '\n'

vault kv put $kvpath "username=$username" "password=$password"
