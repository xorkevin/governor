#!/bin/sh

set -e

export ROOT_DIR=${0%/*}

. "${ROOT_DIR}/_init_lib.sh"

log2 'begin register'

log2 'generate new setup secret'

setupsecret=$(gen_pass "${PASS_LEN:-64}")

while true; do
  i=0
  while [ $i -lt "${CURL_REAUTH:-3}" ]; do
    if [ -z $VAULT_TOKEN ]; then
      export VAULT_TOKEN=$(auth_vault)
      log2 'authenticate with vault'
    fi

    status=$(vault_kvput "$KV_MOUNT" "$KV_PATH_SETUP" "{\"secret\": \"${setupsecret}\"}")
    if is_success "$status"; then
      log2 'write setup secret to vault kv'
      break 2
    fi
    log2 'error write setup secret to vault kv:' "$(cat /tmp/curlres.txt)"

    i=$((i + 1))
    sleep "${CURL_BACKOFF:-5}"
  done
  export VAULT_TOKEN=
done

log2 'generate new token secret'

tokensecret=$(gen_pass "${PASS_LEN:-64}")

while true; do
  i=0
  while [ $i -lt "${CURL_REAUTH:-3}" ]; do
    if [ -z $VAULT_TOKEN ]; then
      export VAULT_TOKEN=$(auth_vault)
      log2 'authenticate with vault'
    fi

    status=$(vault_kvput "$KV_MOUNT" "$KV_PATH_TOKEN" "{\"secret\": \"${tokensecret}\"}")
    if is_success "$status"; then
      log2 'write token secret to vault kv'
      break 2
    fi
    log2 'error write token secret to vault kv:' "$(cat /tmp/curlres.txt)"

    i=$((i + 1))
    sleep "${CURL_BACKOFF:-5}"
  done
  export VAULT_TOKEN=
done

log2 'generate new rsa key'

rsakey=$(gen_rsa "${RSA_BITS:-4096}" | jq -Rs)

while true; do
  i=0
  while [ $i -lt "${CURL_REAUTH:-3}" ]; do
    if [ -z $VAULT_TOKEN ]; then
      export VAULT_TOKEN=$(auth_vault)
      log2 'authenticate with vault'
    fi

    status=$(vault_kvput "$KV_MOUNT" "$KV_PATH_RSA" "{\"secret\": ${rsakey}}")
    if is_success "$status"; then
      log2 'write rsakey to vault kv'
      break 2
    fi
    log2 'error write rsakey to vault kv:' "$(cat /tmp/curlres.txt)"

    i=$((i + 1))
    sleep "${CURL_BACKOFF:-5}"
  done
  export VAULT_TOKEN=
done
