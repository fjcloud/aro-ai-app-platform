# Red Hat diagram review — 5 November 2026

All workshop diagrams now follow the public Red Hat brand conventions and the
static illustration style in the referenced OpenShift architecture guide.
Flat rectangular blocks replace rounded cards. Neutral gray groups, black
primary arrows, blue supporting paths, numbered steps and unmodified official
icons replace the previous custom visual styling. Red Hat Display and Text are
embedded, with no CDN or installed-font dependency.

Antora documentation builds passed for both workshops. Chromium checked 11
standalone SVGs with its network offline: the embedded fonts loaded, all node
text remained inside its bounds, labels stayed on the canvas, and no icon/text
or connector-label/node overlaps were found. The 12 guide placements load,
open their full-size SVG and fit both desktop and mobile layouts. Generation is
byte-reproducible. Vendored source hashes and text contrast ratios were checked.
The browser was launched with `--use-mock-keychain`.

See `validation.json` for individual results and asset hashes. Asset provenance,
licenses, source links and the scope of the public-guideline review are in
`tools/diagram-assets/README.md` and `sources.json`. The login-only internal
technical-diagram templates were not accessed; this is not a claim of formal
Brand team approval. No application or deployment behavior was changed.
