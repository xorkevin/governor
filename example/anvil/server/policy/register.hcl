{{ $ns := .Vars.kube.namespace -}}
{{ $svc := .Vars.kube.service.name -}}
path "{{ .Vars.vault.kvmount }}/data/{{ with .Vars.vault.kvprefix }}{{ . }}/{{ end }}{{ $ns }}/{{ $svc }}/token" {
  capabilities = ["create", "update", "delete", "read", "list"]
}
path "{{ .Vars.vault.kvmount }}/data/{{ with .Vars.vault.kvprefix }}{{ . }}/{{ end }}{{ $ns }}/{{ $svc }}/rsakey" {
  capabilities = ["create", "update", "delete", "read", "list"]
}
