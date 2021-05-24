path "{{ .Vars.vault.dbmount }}/creds/{{ .Vars.kube.namespace }}-{{ .Vars.kube.service.name }}-rw" {
  capabilities = ["read", "list"]
}
