path "{{ .Vars.vault.kvmount }}/data/{{ with .Vars.vault.kvprefix }}{{ . }}/{{ end }}{{ .Vars.kube.namespace }}/{{ .Vars.kube.service.name }}" {
  capabilities = ["read", "list"]
}
