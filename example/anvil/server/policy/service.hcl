{{ $ns := .Vars.kube.namespace -}}
{{ $svc := .Vars.kube.service.name -}}
path "{{ .Vars.vault.kvmount }}/data/{{ with .Vars.vault.kvprefix }}{{ . }}/{{ end }}{{ $ns }}/{{ $svc }}/token" {
  capabilities = ["read", "list"]
}
path "{{ .Vars.vault.kvmount }}/data/{{ with .Vars.vault.kvprefix }}{{ . }}/{{ end }}{{ $ns }}/{{ $svc }}/rsakey" {
  capabilities = ["read", "list"]
}
path "{{ .Vars.vault.kvmount }}/data/{{ with .Vars.vault.kvprefix }}{{ . }}/{{ end }}{{ $ns }}/{{ $svc }}/smtp" {
  capabilities = ["read", "list"]
}
