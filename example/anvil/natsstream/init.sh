#!/bin/sh

set -e

export ROOT_DIR=${0%/*}

. "${ROOT_DIR}/_init_lib.sh"

log2 'begin getpass'

pass=

while true; do
  i=0
  while [ $i -lt "${CURL_REAUTH:-3}" ]; do
    if [ -z $VAULT_TOKEN ]; then
      export VAULT_TOKEN=$(auth_vault)
      log2 'authenticate with vault'
    fi

    status=$(vault_kvget "$KV_MOUNT" "$KV_PATH")
    if is_success "$status"; then
      log2 'found existing vault credentials'
      pass=$(cat /tmp/curlres.txt | jq -r '.data.data.password')
      log2 'use existing password'
      break 2
    else
      log2 'error get vault kv:' "$(cat /tmp/curlres.txt)"
    fi

    i=$((i + 1))
    sleep "${CURL_BACKOFF:-5}"
  done
  export VAULT_TOKEN=
done

cp /etc/natsstream/server.conf /etc/natsstreampass/server.conf
cat <<EOF > /etc/natsstreampass/auth.conf
TOKEN: "${pass}"
EOF
log2 'write password to nats-streaming conf'
