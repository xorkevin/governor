path "{{ .Vars.vault.kvmount }}/data/{{ with .Vars.vault.kvprefix }}{{ . }}/{{ end }}{{ .Vars.kube.namespace }}/{{ .Vars.kube.service.name }}" {
  capabilities = ["create", "update", "delete", "read", "list"]
}
path "{{ .Vars.vault.dbmount }}/config/{{ .Vars.kube.namespace }}-{{ .Vars.kube.service.name }}" {
  capabilities = ["create", "update", "delete", "read", "list"]
}
path "{{ .Vars.vault.dbmount }}/creds/{{ .Vars.kube.namespace }}-{{ .Vars.kube.service.name }}-rw" {
  capabilities = ["create", "update", "delete", "read", "list"]
}
path "{{ .Vars.vault.dbmount }}/creds/{{ .Vars.kube.namespace }}-{{ .Vars.kube.service.name }}-ro" {
  capabilities = ["create", "update", "delete", "read", "list"]
}
