#!/bin/sh

set -e

export ROOT_DIR=${0%/*}

. "${ROOT_DIR}/_init_lib.sh"

log2 'begin register'

pass=$(gen_pass "${PASS_LEN:-64}")

log2 'generate new password'

while true; do
  i=0
  while [ $i -lt "${CURL_REAUTH:-3}" ]; do
    if [ -z $VAULT_TOKEN ]; then
      export VAULT_TOKEN=$(auth_vault)
      log2 'authenticate with vault'
    fi

    status=$(vault_kvput "$KV_MOUNT" "$KV_PATH" "{\"password\": \"${pass}\"}")
    if is_success "$status"; then
      log2 'write password to vault kv'
      break 2
    fi
    log2 'error write password to vault kv:' "$(cat /tmp/curlres.txt)"

    i=$((i + 1))
    sleep "${CURL_BACKOFF:-5}"
  done
  export VAULT_TOKEN=
done
