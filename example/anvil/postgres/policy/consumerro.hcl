{{ $ns := .Vars.kube.namespace -}}
{{ $svc := .Vars.kube.service.name -}}
path "{{ .Vars.vault.dbmount }}/creds/{{ $ns }}-{{ $svc }}-ro" {
  capabilities = ["read", "list"]
}
