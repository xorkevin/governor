#!/bin/sh

set -e

export ROOT_DIR=${0%/*}

. "${ROOT_DIR}/_init_lib.sh"

log2 'begin genpass'

pass=

while true; do
  export VAULT_TOKEN=
  i=0
  while [ $i -lt "${CURL_REAUTH:-12}" ]; do
    if [ -z $VAULT_TOKEN ]; then
      export VAULT_TOKEN=$(auth_vault)
      log2 'authenticate with vault'
    fi

    status=$(vault_kvget "$KV_PATH")
    if is_success "$status"; then
      log2 'found existing vault credentials'
      pass=$(cat /tmp/curlres.txt | jq -r '.data.data.password')
      log2 'use existing password'
      break 2
    elif [ "$status" = 404 ]; then
      log2 'no existing vault credentials'
      if [ -z "$pass" ]; then
        pass=$(gen_pass "${PASS_LEN:-64}")
        log2 'generate new password'
      fi

      status=$(vault_kvput "$KV_PATH" "{\"username\": \"postgres\", \"password\": \"${pass}\"}" 0)
      if is_success "$status"; then
        log2 'write password to vault kv'
        break 2
      fi
      log2 'error write password to vault kv:' "$(cat /tmp/curlres.txt)"
    else
      log2 'error get vault kv:' "$(cat /tmp/curlres.txt)"
    fi

    i=$((i + 1))
    sleep "${CURL_BACKOFF:-5}"
  done
done

printf '%s' "${pass}" > /etc/postgrespass/pass.txt
log2 'write password to postgres conf'
