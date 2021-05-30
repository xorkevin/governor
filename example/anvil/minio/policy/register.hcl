{{ $ns := .Vars.kube.namespace -}}
{{ $svc := .Vars.kube.service.name -}}
path "{{ .Vars.vault.kvmount }}/data/{{ with .Vars.vault.kvprefix }}{{ . }}/{{ end }}{{ $ns }}/{{ $svc }}" {
  capabilities = ["create", "update", "delete", "read", "list"]
}
