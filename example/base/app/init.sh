#!/bin/sh

set -e

export ROOT_DIR=${0%/*}

. "${ROOT_DIR}/_init_lib.sh"

log2 'begin gentokensecret'

secret=

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
      log2 'found existing vault secret'
      secret=$(cat /tmp/curlres.txt | jq -r '.data.data.secret')
      log2 'use existing token secret'
      break 2
    elif [ "$status" = 404 ]; then
      log2 'no existing vault secret'
      if [ -z "$secret" ]; then
        secret=$(gen_pass "${PASS_LEN:-64}")
        log2 'generate new token secret'
      fi

      status=$(vault_kvput "$KV_PATH" "{\"secret\": \"${secret}\"}" 0)
      if is_success "$status"; then
        log2 'write token secret to vault kv'
        break 2
      fi
      log2 'error write token secret to vault kv:' "$(cat /tmp/curlres.txt)"
    else
      log2 'error get vault kv:' "$(cat /tmp/curlres.txt)"
    fi

    i=$((i + 1))
    sleep "${CURL_BACKOFF:-5}"
  done
done

log2 'begin genrsakey'

rsakey=

while true; do
  export VAULT_TOKEN=
  i=0
  while [ $i -lt "${CURL_REAUTH:-12}" ]; do
    if [ -z $VAULT_TOKEN ]; then
      export VAULT_TOKEN=$(auth_vault)
      log2 'authenticate with vault'
    fi

    status=$(vault_kvget "$KV_PATH_RSA")
    if is_success "$status"; then
      log2 'found existing vault rsakey'
      rsakey=$(cat /tmp/curlres.txt | jq -r '.data.data.secret')
      log2 'use existing rsakey'
      break 2
    elif [ "$status" = 404 ]; then
      log2 'no existing vault rsakey'
      if [ -z "$rsakey" ]; then
        rsakey=$(gen_rsa "${RSA_BITS:-4096}" | jq -Rs)
        log2 'generate new rsakey'
      fi

      status=$(vault_kvput "$KV_PATH_RSA" "{\"secret\": ${rsakey}}" 0)
      if is_success "$status"; then
        log2 'write rsakey to vault kv'
        break 2
      fi
      log2 'error write rsakey to vault kv:' "$(cat /tmp/curlres.txt)"
    else
      log2 'error get vault kv:' "$(cat /tmp/curlres.txt)"
    fi

    i=$((i + 1))
    sleep "${CURL_BACKOFF:-5}"
  done
done
