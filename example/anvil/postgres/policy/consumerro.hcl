path "{{ .Vars.vault.dbmount }}/creds/{{ .Vars.kube.namespace }}-{{ .Vars.kube.service.name }}-ro" {
  capabilities = ["read", "list"]
}
