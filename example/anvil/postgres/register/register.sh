#!/bin/sh

set -e

export ROOT_DIR=${0%/*}

. "${ROOT_DIR}/_init_lib.sh"

log2 'begin register'

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
    elif [ "$status" = 404 ]; then
      log2 'no existing vault credentials'
      if [ -z "$pass" ]; then
        pass=$(gen_pass "${PASS_LEN:-64}")
        log2 'generate new password'
      fi

      status=$(vault_kvput "$KV_MOUNT" "$KV_PATH" "{\"username\": \"postgres\", \"password\": \"${pass}\"}")
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
  export VAULT_TOKEN=
done

vault_pgconfig_req() {
  local roles=$1
  local conn=$2
  local password=$3
  cat <<EOF
{
  "plugin_name": "postgresql-database-plugin",
  "allowed_roles": "${roles}",
  "connection_url": "${conn}",
  "username": "postgres",
  "password": "${password}",
  "verify_connection": false
}
EOF
}

while true; do
  i=0
  while [ $i -lt "${CURL_REAUTH:-3}" ]; do
    if [ -z $VAULT_TOKEN ]; then
      export VAULT_TOKEN=$(auth_vault)
      log2 'authenticate with vault'
    fi

    req=$(vault_pgconfig_req "${DB_NAME}-rw,${DB_NAME}-ro" "$DB_CONN" "$pass")
    status=$(vault_dbconfigput "$DB_MOUNT" "$DB_NAME" "$req")
    if is_success "$status"; then
      log2 'write password to vault db engine config'
      break 2
    fi
    log2 'error write password to vault db engine config:' "$(cat /tmp/curlres.txt)"

    i=$((i + 1))
    sleep "${CURL_BACKOFF:-5}"
  done
  export VAULT_TOKEN=
done

vault_pgrole_req() {
  local name=$1
  local creation=$2
  local revocation=$3
  local ttl=$4
  local max_ttl=$5

  local creation_statements=$(cat "$creation" | jq -R | jq -scj)
  local revocation_statements=$(cat "$revocation" | jq -R | jq -scj)
  cat <<EOF
{
  "db_name": "${name}",
  "creation_statements": ${creation_statements},
  "revocation_statements": ${revocation_statements},
  "default_ttl": "${ttl}",
  "max_ttl": "${max_ttl}"
}
EOF
}

while true; do
  i=0
  while [ $i -lt "${CURL_REAUTH:-3}" ]; do
    if [ -z $VAULT_TOKEN ]; then
      export VAULT_TOKEN=$(auth_vault)
      log2 'authenticate with vault'
    fi

    req=$(vault_pgrole_req "$DB_NAME" "${ROOT_DIR}/rolecreate.sql" "${ROOT_DIR}/rolerevoke.sql" "$DB_TTL" "$DB_MAX_TTL")
    status=$(vault_dbroleput "$DB_MOUNT" "${DB_NAME}-rw" "$req")
    if is_success "$status"; then
      log2 'write rw role to vault db engine'
      break 2
    fi
    log2 'error write rw role to vault db engine:' "$(cat /tmp/curlres.txt)"

    i=$((i + 1))
    sleep "${CURL_BACKOFF:-5}"
  done
  export VAULT_TOKEN=
done

while true; do
  i=0
  while [ $i -lt "${CURL_REAUTH:-3}" ]; do
    if [ -z $VAULT_TOKEN ]; then
      export VAULT_TOKEN=$(auth_vault)
      log2 'authenticate with vault'
    fi

    req=$(vault_pgrole_req "${DB_NAME}" "${ROOT_DIR}/rolerocreate.sql" "${ROOT_DIR}/rolerevoke.sql" "$DB_TTL" "$DB_MAX_TTL")
    status=$(vault_dbroleput "$DB_MOUNT" "${DB_NAME}-ro" "$req")
    if is_success "$status"; then
      log2 'write ro role to vault db engine'
      break 2
    fi
    log2 'error write ro role to vault db engine:' "$(cat /tmp/curlres.txt)"

    i=$((i + 1))
    sleep "${CURL_BACKOFF:-5}"
  done
  export VAULT_TOKEN=
done
