#!/usr/bin/env bash
# Live cluster values — never commit a cluster id or apps domain.
# shellcheck disable=SC2034
workshop_apps_domain() {
  oc get ingresses.config.openshift.io cluster -o jsonpath='{.spec.domain}'
}

workshop_cluster_id() {
  oc get infrastructure cluster -o jsonpath='{.status.infrastructureName}'
}

# Wildcard cert on the default IngressController: <cluster-id>-primary-cert-bundle-secret
workshop_maas_cert() {
  echo "$(workshop_cluster_id)-primary-cert-bundle-secret"
}

workshop_maas_host() {
  local h
  h=$(oc get gateway maas-default-gateway -n openshift-ingress \
    -o jsonpath='{.spec.listeners[0].hostname}' 2>/dev/null || true)
  if [[ -n "${h}" && "${h}" != "maas.apps.cluster.example.com" ]]; then
    printf '%s\n' "${h}"
    return
  fi
  printf 'maas.%s\n' "$(workshop_apps_domain)"
}

workshop_maas_base() {
  printf 'https://%s/llm-serving/qwen3/v1\n' "$(workshop_maas_host)"
}
