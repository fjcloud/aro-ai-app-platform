# Red Hat workshop diagram assets

The workshop uses static technical illustrations in SVG, following the public
Red Hat brand standards and the visual conventions of the OpenShift architecture
documentation. RHDS is the design system for web experiences; PatternFly is for
product interfaces. Neither is the renderer for these architecture diagrams.
Red Hat's technical-diagram resources provide Inkscape and Adobe Illustrator
templates. The first architecture image in the reference page records Inkscape
in its PNG software metadata; the second records Adobe ImageReady.

These are workshop illustrations, not official documentation artwork or an
assertion of Brand team approval. The internal technical-diagram templates and
working guide require a Red Hat login and were not available for this review.
The public standards and published diagrams are the verifiable baseline.

## Visual conventions

- Flat, square-cornered rectangles, neutral gray groups and component blocks,
  white highlighted blocks with information-blue outlines; no shadows or gradients.
- Black arrows for primary flows, blue dashed arrows for supporting flows.
  Every path retains an arrowhead and meaningful labels where needed; color
  alone never determines the flow direction or meaning.
- Red Hat Display for headings and node names, Red Hat Text for descriptions.
  Text uses sentence case, left alignment, generous padding, and 1.3–1.5 line spacing.
- Red Hat red (`#ee0000`) identifies the illustration, not an error or failure.
  Neutral grays and information blue (`#0066cc`) follow the Red Hat palette.
- Standard icons depict general concepts at 40 px. Official technology icons
  identify OpenShift Dev Spaces, Pipelines and GitOps. The complete supplied SVGs
  are embedded unchanged, including their original bounding shape and colors.
  No project logos, generated icons, recoloring, cropping, or icon combinations.
- `ui-icons.zip` was reviewed but is intentionally not bundled: UI icons and
  microns are for small interface controls, not diagram artwork.
- SVG title/description, readable text, full-size links and desktop/mobile page
  checks provide access to the diagram independently of its in-page size.

## Regenerate

```sh
python3 tools/generate_workshop_diagrams.py
make docs
```

The generator uses only the Python standard library. Vendored WOFF2 fonts and
SVG icons are embedded as data URIs, so diagrams load without a CDN, local font
installation or network access. Keep text editable rather than converting it
to paths. Both workshop repositories share this renderer and select their
content using Antora metadata.

## Sources and attribution

- [Red Hat Design System](https://ux.redhat.com/)
- [Red Hat technical-diagram resources](https://www.redhat.com/en/about/brand/standards/resources)
- [Reference OpenShift architecture illustrations](https://ocp.msl.cloud/openshift-enterprise/main/architecture/architecture.html)
- [Red Hat typography](https://ux.redhat.com/foundations/typography/font-family/)
- [Typography specifics](https://ux.redhat.com/foundations/typography/type-specifics/)
- [Red Hat colors](https://www.redhat.com/en/about/brand/standards/color)
- [Standard icons](https://www.redhat.com/en/about/brand/standards/icons/standard-icons)
- [Technology icons](https://www.redhat.com/en/about/brand/standards/icons/technology-icons)
- [UI icons](https://www.redhat.com/en/about/brand/standards/icons/ui-icons)

Standard icons: copyright Red Hat, Inc., licensed under
[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/).
See [LICENSE_ICONS.md](LICENSE_ICONS.md). Copies come from the user's official
Brand Portal `icon.zip` archive. Technology icons come from
`technology-icons.zip` and remain unchanged Red Hat brand assets; their
trademarks and applicable brand guidelines are retained. This repository does
not grant a new license to Red Hat trademarks or claim technology icons are
covered by the standard-icon license.

Fonts: copyright Red Hat, Inc., licensed under the SIL Open Font License 1.1.
See [fonts/OFL.txt](fonts/OFL.txt). Unmodified font binaries are pinned to the
[official font repository](https://github.com/RedHatOfficial/RedHatFont/tree/6bb1048a6402b0076ea04f42951ec66263cd1437).

[sources.json](sources.json) records original filenames, source URLs, usage and
SHA-256 hashes. Only assets used by the diagrams are vendored.
