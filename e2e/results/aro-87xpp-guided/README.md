# Guided ARO workshop validation — 87xpp

Validated on 9 November 2026, OpenShift 4.21.22. See [validation.json](validation.json).

The lab already had operator subscriptions and the additional CPU/T4 workers.
The run reused these pools, enabled the platform services, deployed DeepSeek Pro
and D3, and prepared Card Scout and a new developer workspace.

## Verified

- Simplified Act 0: native HTPasswd prompts for both users, existing administrator
  retained, provider reconciliation, named `oc login` and Azure discovery without
  `make onboard`. Fresh browser logins worked for both `workshop-admin` and `dev`.
- Platform instances, governed DeepSeek inference, private T4 inference service,
  developer namespace/RBAC, OGX backend and Card Scout API all became ready.
- `dev` opened Dev Spaces in `dev-devspaces`; the terminal used the scoped
  `workshop-developer` service account, OpenCode 1.18.14 and DeepSeek through MaaS.
- All four documented developer API blocks passed from that workspace. The checks
  verified GPU discovery, Charizard's star, Basic Energy's absent mark, actual
  DeepSeek token usage and a native agent's real catalogue/price tool call.
- As `dev`, the Playground called DeepSeek and `get_card_market_data` successfully.
  The guide's Playground screenshots were refreshed from this session.
- As `workshop-admin`, the external model was Ready in `llm-serving`. The MaaS
  subscription's Models count expanded to show `10,000,000 / 24 hours`.
  Operator, OpenShift AI and MaaS screenshots were captured from these real views.
- 48 local Python regression tests passed; Antora rendering and browser navigation
  of all 16 guide pages passed. Executable shell blocks passed `bash -n`.

## Scope

This validates the revised identity handoff, platform preparation, API preflight
and Playground experience. It is not a new complete application-generation E2E:
code generation, Tekton delivery and Argo CD rollout were not repeated. Existing
worker pools were reused. The gateway/TLS helpers were checked idempotently after
the equivalent manual configuration had succeeded.

Credentials, kubeconfigs, browser sessions and raw model responses stay outside
Git. No privileged SCC or cluster-admin access was granted to `dev`.
