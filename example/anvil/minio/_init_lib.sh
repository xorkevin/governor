#!/bin/sh

log2() {
  printf '[%s] %s\n' "$(date)" "$*" 1>&2
}

is_success() {
  case "$1" in
    2??) return 0;;
    *) return 1;;
  esac
}

read_satoken() {
  cat /var/run/secrets/kubernetes.io/serviceaccount/token
}

auth_vault_req() {
  local satoken=$1
  local role=$2

  cat <<EOF
{ "jwt": "${satoken}", "role": "${role}" }
EOF
}

auth_vault() {
  local satoken=$(read_satoken)
  log2 'read service account token'
  local req=$(auth_vault_req "$satoken" "$VAULT_ROLE")
  curl -s \
    -X POST -d "$req" \
    "${VAULT_ADDR}/v1/auth/${VAULT_KUBE_MOUNT}/login" \
    | jq -r '.auth.client_token'
}

vault_kvput_req() {
  local opts=$1
  local val=$2
  cat <<EOF
{ "options": $opts, "data": $val }
EOF
}

vault_kvput() {
  local mount=$1
  local key=$2
  local val=$3
  local cas=$4

  opts="{}"
  if [ ! -z "$cas" ]; then
    opts="{ \"cas\": $cas }"
  fi

  local req=$(vault_kvput_req "$opts" "$val")
  curl -s -w '%{http_code}' -o /tmp/curlres.txt \
    -H "X-Vault-Token: ${VAULT_TOKEN}" \
    -X POST -d "$req" \
    "${VAULT_ADDR}/v1/${mount}/data/${key}"
}

vault_kvget() {
  local mount=$1
  local key=$2

  curl -s -w '%{http_code}' -o /tmp/curlres.txt \
    -H "X-Vault-Token: ${VAULT_TOKEN}" \
    -X GET \
    "${VAULT_ADDR}/v1/${mount}/data/${key}"
}

vault_dbconfigput() {
  local mount=$1
  local key=$2
  local val=$3

  curl -s -w '%{http_code}' -o /tmp/curlres.txt \
    -H "X-Vault-Token: ${VAULT_TOKEN}" \
    -X POST -d "$val" \
    "${VAULT_ADDR}/v1/${mount}/config/${key}"
}

vault_dbroleput() {
  local mount=$1
  local key=$2
  local val=$3

  curl -s -w '%{http_code}' -o /tmp/curlres.txt \
    -H "X-Vault-Token: ${VAULT_TOKEN}" \
    -X POST -d "$val" \
    "${VAULT_ADDR}/v1/${mount}/roles/${key}"
}

gen_pass() {
  local passlen=$1
  head -c "$passlen" < /dev/urandom | base64 | tr -d '\n=' | tr '+/' '-_'
}

gen_rsa() {
  local bits=$1
  openssl genpkey -algorithm RSA -pkeyopt "rsa_keygen_bits:$bits"
}
