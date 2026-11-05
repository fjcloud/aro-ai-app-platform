# Workshop diagrams — 5 November 2026

The workshop schematics now use standalone SVGs with consistent typography,
spacing, node styles, labeled arrows and full-size links. The source of truth
is `tools/generate_workshop_diagrams.py`; regenerate with Python's standard
library, without image/font downloads. Both workshop copies share the renderer
and select their architecture from Antora metadata.

Antora builds passed. Chromium checked all diagram labels against their node and
canvas bounds, checked edge labels for node overlap, and rendered the diagrams
inside the built guide pages. Images load, full-size links resolve, and diagrams
fit the mobile page width. The generator produces identical bytes on a repeated
run. `validation.json` records the per-diagram/page results and asset checksums.
Browser checks use `--use-mock-keychain`.

ARO depicts Card Scout, external DeepSeek through MaaS, private D3 inference,
read-only agent tools, metrics and MLflow. ROSA keeps its in-cluster Qwen model
and Fortune Cookie story. GitOps diagrams distinguish committed desired state
from the image registry. Obsolete schematic PNGs were removed; current UI
screenshots remain separate from the diagram generator.
