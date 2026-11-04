# Brightside / Darkside — parked experiment

This POC is preserved for later exploration. The ARO workshop continues with
**Card Scout / Pokémon**. This directory has no workshop navigation, setup,
operator, image-publication or application-template integration.

The application listens to new Bluesky posts from 22 general-news publishers.
D3 Mini classifies them as positive, negative, mixed, neutral or uncertain.
Positive candidates above the 65% threshold go to a DeepSeek research agent,
which reads sources, investigates coverage and returns a sourced English summary.
The Dark side view shows original decision-filtered posts, all five model scores,
filter reasons and latency. Filtered posts do not call the research agent.

The runtime was put in standby on 4 November 2026 (`replicas: 0`); its PVC and
historical feed were retained. The manifests here also default to zero replicas.
The lab-specific route and image digest describe the original deployment and
are not a guarantee that its temporary workshop environment still exists.

## Source snapshot

Imported from the standalone local POC at revision `3a5717901a1aa24003b0fc118b891b2a5b74a606`.
Runtime modules, UI, Dockerfile, source accounts, tests and deployment manifests
are retained. `THIRD_PARTY_NOTICES.md` contains the Google News URL-resolution
attribution. Credentials and captured article/post data are not archived here.

Google News discovery is a research tool used after the positive classification;
all feed ingestion comes from the Bluesky WebSocket. Different source domains
are not proof of independent reporting or truth. Unsupported candidates are held
back, with bounded retries.

## Run the isolated POC

Node.js 22+; no npm runtime dependencies. From this directory:

```sh
node --test
LLM_BASE_URL=https://your-endpoint/openai/v1 LLM_MODEL=your-model \
  D3_URL=http://127.0.0.1:18123/v1/systemone node server.mjs
```

Set `LLM_API_KEY` in the process environment before running the server.
`launch.py` can obtain credentials from the original personal Foundry resource
when the operator has appropriate Azure CLI access. The local URL defaults to
`http://localhost:18124`; `?view=dark` opens the filtered feed. `AUTO_START=0`
starts paused. State is stored under the ignored `data/` directory.

## Resume on a cluster

`deploy/` contains the original isolated build namespace, application namespace,
PVC, Secret reference, HTTPS route, restricted-v2 pod and narrow D3 access rule.
Provision the LLM Secret separately. For another environment, rebuild the image
and update its immutable reference, route host and matching `PUBLIC_ORIGIN`.
No new GPU or model replica is required.

On the original lab, after reviewing the configuration:

```sh
oc apply -k deploy
oc scale deployment/brightside -n brightside-poc-dev --replicas=1
```

Return to standby with `--replicas=0`. One replica and the Recreate strategy
prevent concurrent ingestion engines from writing the same persisted state.
See `VALIDATION.md` for the historical POC acceptance results.
