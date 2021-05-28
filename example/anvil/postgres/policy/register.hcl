{{ $ns := .Vars.kube.namespace -}}
{{ $svc := .Vars.kube.service.name -}}
{{ $nssvc := printf "%s-%s" $ns $svc -}}
path "{{ .Vars.vault.kvmount }}/data/{{ with .Vars.vault.kvprefix }}{{ . }}/{{ end }}{{ .Vars.kube.namespace }}/{{ .Vars.kube.service.name }}" {
  capabilities = ["create", "update", "delete", "read", "list"]
}
path "{{ .Vars.vault.dbmount }}/config/{{ $nssvc }}" {
  capabilities = ["create", "update", "delete", "read", "list"]
}
path "{{ .Vars.vault.dbmount }}/creds/{{ $nssvc }}-rw" {
  capabilities = ["create", "update", "delete", "read", "list"]
}
path "{{ .Vars.vault.dbmount }}/creds/{{ $nssvc }}-ro" {
  capabilities = ["create", "update", "delete", "read", "list"]
}
path "{{ .Vars.vault.dbmount }}/roles/{{ $nssvc }}-rw" {
  capabilities = ["create", "update", "delete", "read", "list"]
}
path "{{ .Vars.vault.dbmount }}/roles/{{ $nssvc }}-ro" {
  capabilities = ["create", "update", "delete", "read", "list"]
}
