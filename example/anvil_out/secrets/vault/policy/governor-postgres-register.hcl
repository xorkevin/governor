path "kv/data/infra/governor/postgres" {
  capabilities = ["create", "update", "delete", "read", "list"]
}
path "database/config/governor-postgres" {
  capabilities = ["create", "update", "delete", "read", "list"]
}
path "database/creds/governor-postgres-rw" {
  capabilities = ["create", "update", "delete", "read", "list"]
}
path "database/creds/governor-postgres-ro" {
  capabilities = ["create", "update", "delete", "read", "list"]
}
path "database/roles/governor-postgres-rw" {
  capabilities = ["create", "update", "delete", "read", "list"]
}
path "database/roles/governor-postgres-ro" {
  capabilities = ["create", "update", "delete", "read", "list"]
}
