{{ $ns := .Vars.kube.namespace -}}
{{ $svc := .Vars.kube.service.name -}}
path "{{ .Vars.vault.kvmount }}/data/{{ with .Vars.vault.kvappprefix }}{{ . }}/{{ end }}{{ $ns }}/{{ $svc }}/setup" {
  capabilities = ["read", "list"]
}
path "{{ .Vars.vault.kvmount }}/data/{{ with .Vars.vault.kvappprefix }}{{ . }}/{{ end }}{{ $ns }}/{{ $svc }}/token" {
  capabilities = ["read", "list"]
}
path "{{ .Vars.vault.kvmount }}/data/{{ with .Vars.vault.kvappprefix }}{{ . }}/{{ end }}{{ $ns }}/{{ $svc }}/rsakey" {
  capabilities = ["read", "list"]
}
path "{{ .Vars.vault.kvmount }}/data/{{ with .Vars.vault.kvappprefix }}{{ . }}/{{ end }}{{ $ns }}/{{ $svc }}/smtp" {
  capabilities = ["read", "list"]
}
