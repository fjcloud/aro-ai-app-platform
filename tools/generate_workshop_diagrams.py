#!/usr/bin/env python3
"""Generate the workshop's accessible, standalone SVG diagrams (standard library only).

Run from either the ARO or ROSA checkout. Platform selection comes from Antora
metadata, so output is reproducible in arbitrary checkout/worktree directories.
Official icons and Red Hat fonts are vendored with attribution and embedded in
each SVG. No network, installed fonts, image libraries or browser are needed.
"""
from __future__ import annotations

import html
import base64
import json
import math
import re
import textwrap
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'content/modules/ROOT/assets/images'
PLATFORM = 'rosa' if 'ROSA' in (ROOT / 'content/antora.yml').read_text() else 'aro'
ASSETS = ROOT / 'tools/diagram-assets'
RED, INK, MUTED, BORDER, PALE, BLUE = '#ee0000', '#151515', '#4d4d4d', '#707070', '#f2f2f2', '#0066cc'
W = 1120


def esc(value):
    return html.escape(str(value), quote=True)


def data_uri(path, media):
    return f'data:{media};base64,' + base64.b64encode(path.read_bytes()).decode('ascii')


def node_icon(title):
    """Use offering icons only for the named offering; standard icons otherwise."""
    title = title.lower()
    for words, name in [
        (('dev spaces',), 'devspaces'), (('tekton', 'openshift pipelines'), 'pipelines'),
        (('argo cd', 'openshift gitops'), 'gitops'), (('registry',), 'registry'),
        (('opencode', 'curl'), 'code'), (('credential', 'provider key'), 'key'),
        (('maas', 'gateway', 'internal service'), 'gateway'),
        (('foundry', 'deepseek'), 'cloud'),
        (('d3', 'qwen', 'llminferenceservice'), 'model'),
        (('agent',), 'inference'), (('mcp', 'check price'), 'search'),
        (('metrics', 'monitoring', 'dashboards', 'capacity', 'quota', 'governance'), 'metrics'),
        (('mlflow', 'trace'), 'flowchart'),
        (('images', 'source image', 'bottom-right'), 'image'),
        (('repository',), 'data'),
        (('app', 'platform', 'browser', 'feed', 'live'), 'website'),
    ]:
        if any(word in title for word in words):
            return name
    return None


class Diagram:
    def __init__(self, title, subtitle, height=800):
        self.title, self.subtitle, self.height = title, subtitle, height
        self.parts, self.nodes = [], {}
        self.parts.append(f'<rect width="{W}" height="{height}" fill="white"/>')
        self.parts.append(f'<path d="M 40,32 H 1080" stroke="{RED}" stroke-width="3"/>')
        self.text(40, 77, title, size=30, weight=700, cls='diagram-title')
        self.text(40, 108, subtitle, size=18, fill=MUTED)

    def text(self, x, y, value, size=18, fill=INK, weight=400, anchor='start', cls=''):
        self.parts.append(f'<text x="{x}" y="{y}" font-size="{size}" fill="{fill}" font-weight="{weight}" text-anchor="{anchor}" class="{cls}">{esc(value)}</text>')

    def rect(self, x, y, w, h, fill='white', stroke=BORDER, radius=0, dashed=False):
        dash = ' stroke-dasharray="7 5"' if dashed else ''
        self.parts.append(f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{radius}" fill="{fill}" stroke="{stroke}" stroke-width="1.5"{dash}/>')

    def group(self, x, y, w, h, label, external=False, label_x=None):
        self.rect(x, y, w, h, fill='white' if external else PALE, stroke=BORDER if external else 'none', dashed=external)
        step = re.match(r'^(\d+)\s+(.*)$', label)
        if step:
            self.parts.append(f'<circle cx="{x+26}" cy="{y+24}" r="12" fill="{INK}"/>')
            self.text(x+26, y+30, step[1], size=16, fill='white', anchor='middle', weight=700, cls='group-title')
            label, label_x = step[2], x + 48
        self.text(x + 20 if label_x is None else label_x, y + 30, label, size=18, fill=INK, weight=700, cls='group-title')

    def node(self, key, x, y, title, lines=(), w=260, h=140, accent=False):
        self.nodes[key] = (x, y, w, h)
        self.parts.append(f'<g data-role="node" data-node="{esc(key)}">')
        self.rect(x, y, w, h, fill='white' if accent else '#e0e0e0', stroke=BLUE if accent else 'none')
        icon = node_icon(title)
        title_x = x + (72 if icon else 20)
        titles = textwrap.wrap(title, width=max(12, int((w - (92 if icon else 40)) / 10.4)), break_long_words=False)
        body = [line for value in lines for line in textwrap.wrap(value, width=max(18, int((w - 40) / 8.7)), break_long_words=False)]
        header = max(40 if icon else 0, len(titles) * 26)
        required = header + len(body) * 24 + (10 if body else 0)
        if required > h - 22:
            raise ValueError(f'Node too small: {key}: {title} ({required} in {h})')
        top = y + (h - required) / 2
        if icon:
            uri = data_uri(ASSETS / 'icons' / (icon + '.svg'), 'image/svg+xml')
            self.parts.append(f'<image data-role="icon" data-icon="{icon}" x="{x+18}" y="{top+(header-40)/2}" width="40" height="40" href="{uri}" aria-hidden="true"/>')
        baseline = top + (header - len(titles)*26)/2 + 20
        for line in titles:
            self.text(title_x, baseline, line, size=20, weight=500, cls='node-title')
            baseline += 26
        top += header + (10 if body else 0) + 18
        for line in body:
            self.text(x + 20, top, line, size=18, fill=INK, cls='node-body')
            top += 24
        self.parts.append('</g>')

    def port(self, key, side):
        x, y, w, h = self.nodes[key]
        return {'l': (x, y + h / 2), 'r': (x + w, y + h / 2),
                't': (x + w / 2, y), 'b': (x + w / 2, y + h)}[side]

    def edge(self, source, target, via=(), label=None, at=None, support=False):
        start, end = self.port(*source), self.port(*target)
        points = [start, *via, end]
        color = BLUE if support else INK
        dash = ' stroke-dasharray="6 5"' if support else ''
        path = 'M ' + ' L '.join(f'{x},{y}' for x, y in points)
        self.parts.append(f'<path data-role="connector" data-source="{source[0]}" data-target="{target[0]}" data-points="{esc(json.dumps(points))}" d="{path}" fill="none" stroke="{color}" stroke-width="2.2" stroke-linejoin="round" marker-end="url(#{"blue" if support else "arrow"})"{dash}/>')
        if label:
            x, y = at or ((start[0] + end[0]) / 2, (start[1] + end[1]) / 2 - 14)
            width = len(label) * 8.7 + 18
            self.parts.append('<g data-role="edge-label">')
            self.rect(x - width / 2, y - 18, width, 26, stroke='none')
            self.text(x, y + 1, label, size=17, fill=color, anchor='middle')
            self.parts.append('</g>')

    def footer(self, message):
        y = self.height - 56
        self.parts.append(f'<path d="M 40,{y} H 1080" stroke="{BORDER}"/>')
        self.text(40, y + 31, message, size=16, fill=MUTED)

    def save(self, name):
        OUT.mkdir(parents=True, exist_ok=True)
        defs = ''.join(f'<marker id="{key}" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0,0 L 10,5 L 0,10 z" fill="{color}"/></marker>' for key, color in [('arrow', INK), ('blue', BLUE)])
        fonts = ''.join(f"@font-face{{font-family:'{family}';font-style:normal;font-weight:{weight};src:url('{data_uri(ASSETS / 'fonts' / file, 'font/woff2')}') format('woff2')}}" for family, weight, file in [('Red Hat Display', '300 900', 'RedHatDisplay-VF.woff2'), ('Red Hat Text', '400', 'RedHatText-Regular.woff2')])
        body = '\n'.join(self.parts)
        svg = f'''<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{self.height}" viewBox="0 0 {W} {self.height}" role="img" aria-labelledby="title desc">
<title id="title">{esc(self.title)}</title><desc id="desc">{esc(self.subtitle)}</desc>
<metadata>Workshop illustration using Red Hat brand conventions. Red Hat standard icons: copyright Red Hat, Inc., CC-BY-4.0 (https://creativecommons.org/licenses/by/4.0/). Technology icons: official Red Hat brand assets used unchanged; trademarks retained. Red Hat fonts: copyright Red Hat, Inc., SIL Open Font License 1.1. Asset provenance: tools/diagram-assets/sources.json. Not an official Red Hat documentation illustration or brand approval.</metadata>
<defs>{defs}</defs><style>{fonts}text{{font-family:'Red Hat Text',sans-serif}}.diagram-title,.node-title,.group-title{{font-family:'Red Hat Display',sans-serif}}svg{{background:white}}</style>
{body}
</svg>\n'''
        (OUT / name).write_text(svg)
        print(name)


def architecture():
    if PLATFORM == 'aro':
        d = Diagram('Card Scout on ARO', 'Code with a managed LLM. Classify images locally. Verify prices through an agent.', 920)
        rows = [
            (145, '1  Build the Go application', [('OpenShift Dev Spaces', ['OpenCode + app contract', 'AGENTS.md']), ('OpenShift AI MaaS', ['Coding key + token quota']), ('DeepSeek on Foundry', ['Managed model in Azure'])]),
            (375, '2  Analyze the collection', [('Browser + Go proxy', ['One prepared platform API']), ('Card Scout platform', ['Fixed images, state + rules']), ('d3-mini on one T4', ['Card kind + rarity symbol'])]),
            (605, '3  Verify a selected card', [('Card Scout platform', ['Separate runtime MaaS key']), ('OpenShift AI agent', ['DeepSeek through MaaS']), ('Read-only MCP tool', ['TCGdex identity + prices'])]),
        ]
        for n, (y, label, boxes) in enumerate(rows):
            d.group(20, y, 1080, 210, label)
            for j, (title, lines) in enumerate(boxes):
                d.node(f'{n}-{j}', 60 + j * 370, y + 49, title, lines, h=145, accent=j == 1)
            d.edge((f'{n}-0', 'r'), (f'{n}-1', 'l'))
            d.edge((f'{n}-1', 'r'), (f'{n}-2', 'l'))
        d.footer('The Go app has no model key. MLflow records model decisions and actual tool evidence.')
    else:
        d = Diagram('AI-powered development on ROSA', 'One in-cluster coding model. A governed API. A repeatable delivery path.', 850)
        d.group(20, 145, 1080, 205, 'Model access inside the ROSA cluster')
        for key, x, title, body in [('dev', 60, 'OpenShift Dev Spaces', ['OpenCode + AGENTS.md']), ('maas', 430, 'OpenShift AI MaaS', ['API key + token quota']), ('model', 800, 'Qwen3.8-27B INT4', ['Red Hat vLLM on one L40S'])]:
            d.node(key, x, 200, title, body, h=128, accent=key == 'maas')
        d.edge(('dev', 'r'), ('maas', 'l')); d.edge(('maas', 'r'), ('model', 'l'))
        d.group(20, 390, 1080, 230, 'Commit, build and deliver')
        for key, x, title, body in [('git', 60, 'Personal Git repository', ['External workshop service']), ('build', 430, 'OpenShift Pipelines', ['Tekton + image registry']), ('app', 800, 'Fortune Cookie app', ['Go application on ROSA'])]:
            d.node(key, x, 445, title, body, h=145, accent=key == 'app')
        d.edge(('git', 'r'), ('build', 'l'), label='clone'); d.edge(('build', 'r'), ('app', 'l'), label='image')
        d.edge(('git', 'b'), ('app', 'b'), via=[(190, 690), (930, 690)], label='Argo CD reconciles committed manifests', at=(560, 724), support=True)
        d.footer('The LLM writes the app. Fortune Cookie serves local strings; it makes no runtime model calls.')
    d.save('architecture-overview.svg')


def serving():
    if PLATFORM == 'aro':
        d = Diagram('Local image inference on one T4', 'OpenShift AI / KServe manages the model. Allowed callers use a private service.', 650)
        d.group(20, 150, 1080, 390, 'ARO cluster')
        d.group(410, 205, 670, 270, 'decision-serving')
        d.node('clients', 60, 280, 'Allowed callers', ['Card Scout platform', 'Dev Spaces'], w=260, h=145)
        d.node('service', 440, 280, 'Internal service', ['NetworkPolicy', 'No API key needed', 'No public Route'], w=280, h=145, accent=True)
        d.node('model', 800, 280, 'd3-mini', ['Native runtime on one T4', 'Weights cached on PVC'], h=145)
        d.edge(('clients', 'r'), ('service', 'l'), label='images'); d.edge(('service', 'r'), ('model', 'l'))
        d.text(60, 514, 'Wait for InferenceService / Ready. GPU-node readiness alone is not model readiness.', size=18, fill=MUTED)
        d.footer('D3 classifies images inside ARO. NetworkPolicy limits access to its internal service.')
    else:
        d = Diagram('Qwen3.8 as Models-as-a-Service', 'A governed OpenAI-compatible endpoint for the in-cluster coding model.', 720)
        d.group(20, 150, 1080, 460, 'ROSA cluster')
        d.node('client', 60, 230, 'OpenCode / curl', ['HTTPS + MaaS API key'], h=145)
        d.node('maas', 430, 230, 'MaaS gateway', ['Authentication', 'Token allowance'], h=145, accent=True)
        d.node('model', 800, 230, 'Qwen3.8 model', ['LLMInferenceService', 'API id: qwen3', 'Red Hat vLLM on L40S'], h=145)
        d.edge(('client', 'r'), ('maas', 'l')); d.edge(('maas', 'r'), ('model', 'l'))
        d.node('dashboard', 430, 460, 'OpenShift AI dashboard', ['Catalog, subscriptions + keys'], h=145)
        d.edge(('dashboard', 't'), ('maas', 'b'), label='manage', at=(670, 420), support=True)
        d.footer('MaaS governs access; model inference runs on the ROSA cluster, not at a cloud LLM provider.')
    d.save('model-serving-architecture.svg')


def cicd():
    app = 'Card Scout' if PLATFORM == 'aro' else 'Fortune Cookie'
    d = Diagram('From prompt to a running application', 'Git supplies the source and desired state. The registry supplies the built image.', 870)
    coding = 'Generate from the contract' if PLATFORM == 'aro' else 'Generate with AGENTS.md'
    for key, x, title, body in [('code', 60, 'OpenCode', [coding]), ('git', 430, 'Personal Git repository', ['Commit source + manifests']), ('tekton', 800, 'OpenShift Pipelines', ['Tekton: clone and build'])]:
        d.node(key, x, 165, title, body, h=145, accent=key == 'git')
    d.edge(('code', 'r'), ('git', 'l'), label='push'); d.edge(('git', 'r'), ('tekton', 'l'), label='clone')
    d.group(20, 365, 1080, 420, 'Reconcile, pull and expose', label_x=420)
    d.node('argo', 60, 420, 'OpenShift GitOps', ['Argo CD reconciles Git'], h=130)
    d.node('deploy', 430, 420, 'App Deployment', ['Source and image verified'], h=130, accent=True)
    d.node('registry', 800, 420, 'Internal image registry', ['Built application image'], h=130)
    d.edge(('git', 'b'), ('argo', 't'), via=[(560, 340), (190, 340)], label='desired state', at=(350, 329))
    d.edge(('tekton', 'b'), ('registry', 't'), label='push image', at=(1005, 349))
    d.edge(('argo', 'r'), ('deploy', 'l'), label='apply')
    d.edge(('registry', 'l'), ('deploy', 'r'), label='image')
    d.node('route', 430, 655, app + ' live', ['Public HTTPS Route'], h=120)
    d.edge(('deploy', 'b'), ('route', 't'), label='serve', at=(625, 605))
    d.footer('Delivery is complete when Git revision, Argo CD health and the running image digest agree.')
    d.save('cicd-gitops.svg')


def observability():
    aro = PLATFORM == 'aro'
    d = Diagram('Observe usage and explain behavior', 'Keep operational signals, model evidence and cloud billing distinct.', 790)
    d.group(20, 145, 1080, 220, 'Metrics — load, capacity and consumption')
    sources = ['D3 + DCGM + MaaS'] if aro else ['DCGM + vLLM + MaaS']
    for key, x, title, body in [('metrics', 60, 'Platform metrics', sources), ('prom', 430, 'OpenShift monitoring', ['Prometheus + ServiceMonitors']), ('console', 800, 'Console dashboards', ['GPU, requests and tokens'])]:
        d.node(key, x, 205, title, body, h=145, accent=key == 'prom')
    d.edge(('metrics', 'r'), ('prom', 'l')); d.edge(('prom', 'r'), ('console', 'l'))
    if aro:
        d.group(20, 410, 1080, 220, 'Traces — inspect model and tool evidence')
        for key, x, title, body in [('spans', 60, 'Card Scout platform', ['Model + MCP tool spans']), ('mlflow', 430, 'MLflow', ['Parent / child traces']), ('graph', 800, 'Trace graph', ['Raw symbol + mapped rarity', 'Tool inputs + outputs'])]:
            d.node(key, x, 470, title, body, h=145, accent=key == 'mlflow')
        d.edge(('spans', 'r'), ('mlflow', 'l'), support=True); d.edge(('mlflow', 'r'), ('graph', 'l'), support=True)
    else:
        d.node('capacity', 60, 440, 'Provisioned capacity', ['GPU workers + storage', 'Still billed while idle'], w=480, h=170)
        d.node('quota', 580, 440, 'Token governance', ['MaaS limits consumption', 'It is not an AWS spending cap'], w=480, h=170)
    d.footer('Metrics show usage. Traces explain decisions. Use provider billing for actual infrastructure spend.' if aro else 'Compare demand, GPU capacity and quota. Use AWS billing for actual infrastructure spend.')
    d.save('the-bill-observability.svg')


def maas_access():
    d = Diagram('Two credentials, two trust boundaries', 'Clients use MaaS credentials. Only the platform holds the Azure provider credential.', 730)
    d.group(20, 155, 710, 440, 'ARO cluster')
    d.group(780, 155, 320, 440, 'Azure Foundry', external=True)
    d.node('client', 55, 230, 'Model client', ['OpenCode / native agent', 'Uses a MaaS key'], w=255, h=150)
    d.node('maas', 425, 230, 'OpenShift AI MaaS', ['Authenticate + enforce quota'], w=275, h=150, accent=True)
    d.node('provider', 800, 230, 'DeepSeek', ['Managed Azure endpoint'], w=280, h=150)
    d.edge(('client', 'r'), ('maas', 'l'), label='MaaS key', at=(367, 279))
    d.edge(('maas', 'r'), ('provider', 'l'), label='Azure key', at=(750, 279))
    d.node('secret', 425, 455, 'Provider key', ['Cluster Secret', 'Never sent to workspaces'], w=275, h=135)
    d.edge(('secret', 't'), ('maas', 'b'), label='read', at=(625, 421))
    d.footer('Coding and runtime callers have separate MaaS subscriptions and API keys.')
    d.save('maas-access.svg')


def workflow():
    d = Diagram('Card Scout: analyze, then verify', 'The generated Go app calls a fixed platform API; models and credentials stay on the platform.', 760)
    rows = [(145, '1  Analyze the batch', [('105 source images', ['102 Pokémon + 3 controls']), ('D3 vision decisions', ['Card kind + rarity symbol']), ('Ranked card feed', ['Common, Uncommon, Rare'])]),
            (410, '2  Check a selected Pokémon card', [('Select Check price', ['One eligible card at a time']), ('Native OpenShift AI agent', ['DeepSeek through MaaS']), ('Read-only MCP tool', ['TCGdex catalogue + prices'])])]
    for n, (y, label, nodes) in enumerate(rows):
        d.group(20, y, 1080, 220, label)
        for j, (title, body) in enumerate(nodes):
            d.node(f'{n}-{j}', 60 + 370*j, y+60, title, body, h=135, accent=j == 1)
        d.edge((f'{n}-0', 'r'), (f'{n}-1', 'l')); d.edge((f'{n}-1', 'r'), (f'{n}-2', 'l'))
    d.footer('The agent returns verified identity, dated price references and sources. MLflow keeps the evidence.')
    d.save('card-scout-workflow.svg')


def rarity():
    d = Diagram('Read the mark, then map the rarity', 'One D3 request; card-kind eligibility gates the rarity shown in the application.', 820)
    d.node('image', 40, 170, 'Source image', ['102 Base Set cards', '3 negative controls'], w=260, h=140)
    d.node('kind', 430, 170, 'Pokémon TCG card?', ['Recognize the printed layout'], w=260, h=140, accent=True)
    d.node('mark', 820, 170, 'Bottom-right mark', ['Read the small symbol', 'By the card number'], w=260, h=140)
    d.edge(('image', 'r'), ('kind', 'l'), label='pixels'); d.edge(('kind', 'r'), ('mark', 'l'), label='yes')
    d.node('other', 430, 440, 'Other / needs review', ['No Pokémon rarity', 'No Pokémon price lookup'], w=260, h=150)
    d.edge(('kind', 'b'), ('other', 't'), label='no / unknown', at=(645, 375))
    d.nodes['mapping'] = (800, 415, 300, 275)
    d.parts.append('<g data-role="node" data-node="mapping">')
    d.rect(800, 415, 300, 275)
    d.text(820, 450, 'Base Set mapping', size=22, weight=650)
    for j, (symbol, label) in enumerate([('circle', 'Common'), ('diamond', 'Uncommon'), ('star', 'Rare'), ('none', 'Common*'), ('uncertain', 'Uncertain')]):
        y = 488 + j * 39
        if symbol == 'circle':
            d.parts.append(f'<circle cx="831" cy="{y-6}" r="6" fill="{INK}"/>')
        elif symbol == 'diamond':
            d.parts.append(f'<path d="M831,{y-14} l8,8 -8,8 -8,-8 z" fill="{INK}"/>')
        elif symbol == 'star':
            points = [(831 + (10 if i%2 == 0 else 4)*math.sin(i*math.pi/5), y-6-(10 if i%2 == 0 else 4)*math.cos(i*math.pi/5)) for i in range(10)]
            d.parts.append('<polygon points="'+' '.join(f'{x},{z}' for x,z in points)+f'" fill="{INK}"/>')
        else:
            d.text(831, y, '∅' if symbol == 'none' else '?', size=22, anchor='middle', weight=600)
        d.text(855, y, label, size=19)
    d.parts.append('</g>')
    d.edge(('mark', 'b'), ('mapping', 't'))
    d.text(40, 650, 'Ignore large Energy icons, artwork and the 1st Edition stamp.', size=18, fill=MUTED)
    d.text(40, 683, 'A diamond is a square rotated 45°. An unreadable corner is not an absent mark.', size=18, fill=MUTED)
    d.footer('* No mark → Common applies only to this known Base Set, with a fully visible corner.')
    d.save('rarity-decision.svg')


def main():
    architecture(); serving(); cicd(); observability()
    if PLATFORM == 'aro':
        maas_access(); workflow(); rarity()


if __name__ == '__main__':
    main()
