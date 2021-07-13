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

log2 'find otp encryption keys'

otpsecrets='[]'

while true; do
  i=0
  while [ $i -lt "${CURL_REAUTH:-3}" ]; do
    if [ -z $VAULT_TOKEN ]; then
      export VAULT_TOKEN=$(auth_vault)
      log2 'authenticate with vault'
    fi

    status=$(vault_kvget "$KV_MOUNT" "$KV_PATH_OTP")
    if is_success "$status"; then
      log2 'found existing vault kv'
      otpsecrets=$(cat /tmp/curlres.txt | jq -c '.data.data.secrets')
      print "$otpsecrets" | jq 'if type != "array" then error("secrets is not an array") else "secrets is array" end'
      log2 'have' $(print "$otpsecrets" | jq 'length') 'otp encryption keys'
      break 2
    elif [ "$status" = 404 ]; then
      log2 'no existing otp encryption keys'
      break 2
    else
      log2 'error get vault kv:' "$(cat /tmp/curlres.txt)"
    fi

    i=$((i + 1))
    sleep "${CURL_BACKOFF:-5}"
  done
  export VAULT_TOKEN=
done

log2 'generate new otp encryption key'

otpsecret=$(gen_pass "${PASS_LEN:-64}")
nextotpsecrets=$(printf '["$cc20p$%s"] %s' "$otpsecret" "$otpsecrets" | jq -sc 'add')

log2 'uploading' $(print "$nextotpsecrets" | jq 'length') 'otp encryption keys'

while true; do
  i=0
  while [ $i -lt "${CURL_REAUTH:-3}" ]; do
    if [ -z $VAULT_TOKEN ]; then
      export VAULT_TOKEN=$(auth_vault)
      log2 'authenticate with vault'
    fi

    status=$(vault_kvput "$KV_MOUNT" "$KV_PATH_OTP" "{\"secrets\": ${nextotpsecrets}}")
    if is_success "$status"; then
      log2 'write otp encryption keys to vault kv'
      break 2
    fi
    log2 'error write otp encryption keys to vault kv:' "$(cat /tmp/curlres.txt)"

    i=$((i + 1))
    sleep "${CURL_BACKOFF:-5}"
  done
  export VAULT_TOKEN=
done

log2 'done registering'
