#!/bin/sh

namespace=governor
dir=base/app

flightctl connect postgres -n $namespace -o $dir $namespace postgres
flightctl connect redis -n $namespace -o $dir $namespace redis
flightctl connect minio -n $namespace -o $dir $namespace minio
flightctl connect nats -n $namespace -o $dir $namespace nats
flightctl connect natsstream -n $namespace -o $dir $namespace natsstream
cat <<EOF > "${dir}/policy/governor.policy.hcl"
path "kv/data/app/governor/token" {
  capabilities = ["create", "update", "read"]
}
path "kv/data/external/smtp" {
  capabilities = ["read"]
}
EOF
flightctl connect vault -n $namespace -k $dir governor
flightctl connect kube -n $namespace -o $dir governor
