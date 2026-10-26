#!/usr/bin/env python3
"""Read-only assertions for the MaaS-governed Foundry ARO workshop (one inference call)."""
import base64
import html
import json
import os
import subprocess
import sys
import urllib.error
import urllib.request


def oc(*args):
    return subprocess.check_output(['oc', *args], text=True, stderr=subprocess.DEVNULL, timeout=45).strip()


def resource(kind, name, namespace=None):
    args = ['get', kind, name, '-o', 'json']
    if namespace:
        args += ['-n', namespace]
    return json.loads(oc(*args))


def require(condition, message):
    if not condition:
        raise AssertionError(message)
    print('PASS:', message, flush=True)


def completion():
    secret = resource('secret', 'ai-provider-openai-api-key', 'openshift-operators')['data']
    endpoint = base64.b64decode(secret['OPENAI_BASE_URL']).decode().rstrip('/')
    key = base64.b64decode(secret['OPENAI_API_KEY']).decode()
    require(endpoint == 'https://' + resource('gateway', 'maas-default-gateway', 'openshift-ingress')['spec']['listeners'][0]['hostname'] + '/v1', 'Workspace uses the MaaS HTTPS base URL')
    request = urllib.request.Request(endpoint + '/chat/completions', data=json.dumps({
        'model': 'deepseekv4pro', 'messages': [{'role': 'user', 'content': 'Reply with ARO_WORKSHOP_OK.'}],
        'max_completion_tokens': 256,
    }).encode(), headers={'Content-Type': 'application/json', 'Authorization': 'Bearer ' + key})
    with urllib.request.urlopen(request, timeout=180) as response:
        result = json.load(response)
        effective_tpm = response.headers.get('x-ratelimit-limit-tokens')
    require(bool(result['choices'][0]['message'].get('content')), 'MaaS routed a real Foundry completion')
    usage = result.get('usage', {})
    require(usage.get('prompt_tokens', 0) > 0 and usage.get('completion_tokens', 0) > 0, 'Foundry returned measured token usage')
    print('Token usage:', json.dumps(usage))
    if effective_tpm:
        print('Effective Foundry tokens/minute:', effective_tpm)
    return usage


def platform():
    require(oc('auth', 'can-i', '*', '*', '--all-namespaces') == 'yes', 'Cluster administrator access')
    workers = [n for n in json.loads(oc('get', 'nodes', '-o', 'json'))['items']
               if 'node-role.kubernetes.io/worker' in n['metadata'].get('labels', {})]
    require(bool(workers) and all(any(c['type'] == 'Ready' and c['status'] == 'True' for c in n['status']['conditions']) for n in workers), 'All workers are Ready')
    for ns, name in [('openshift-operators', 'devspaces-operator'), ('openshift-gitops', 'openshift-gitops-server'), ('openshift-pipelines', 'tekton-pipelines-controller'), ('redhat-ods-operator', 'rhods-operator'), ('kuadrant-system', 'kuadrant-operator-controller-manager'), ('cert-manager', 'cert-manager'), ('openshift-lws-operator', 'openshift-lws-operator'), ('openshift-nfd', 'nfd-controller-manager'), ('nvidia-gpu-operator', 'gpu-operator')]:
        d = resource('deployment', name, ns)
        require(any(c['type'] == 'Available' and c['status'] == 'True' for c in d.get('status', {}).get('conditions', [])), name + ' available')
    for name in ['workshop-git-clone', 'workshop-buildah']:
        resource('task', name, 'openshift-pipelines')
        print('PASS:', name, 'installed')
    che = resource('checluster', 'devspaces', 'openshift-operators')
    require(che.get('status', {}).get('chePhase') == 'Active', 'Dev Spaces Active')
    cm = resource('configmap', 'opencode-workspace-config', 'openshift-operators')
    require(json.loads(cm['data']['opencode.json'])['model'] == 'deepseek/deepseekv4pro', 'Platform pins the MaaS-published DeepSeek model')
    for kind, name, ns in [('externalprovider.inference.opendatahub.io', 'foundry', 'llm-serving'),
                            ('externalmodel.inference.opendatahub.io', 'deepseekv4pro', 'llm-serving'),
                            ('maasmodelref', 'deepseekv4pro', 'llm-serving')]:
        require(resource(kind, name, ns).get('status', {}).get('phase') == 'Ready', kind + ' Ready')
    for kind, name in [('maasauthpolicy', 'deepseekv4pro-access'), ('maassubscription', 'deepseekv4pro-workshop')]:
        require(resource(kind, name, 'models-as-a-service').get('status', {}).get('phase') == 'Active', kind + ' Active')
    provider = resource('secret', 'foundry-provider-key', 'llm-serving')
    workspace = resource('secret', 'ai-provider-openai-api-key', 'openshift-operators')
    key = base64.b64decode(workspace['data']['OPENAI_API_KEY']).decode()
    require(key.startswith('sk-oai-') and key != base64.b64decode(provider['data']['api-key']).decode(), 'Workspace has a separate MaaS key, not the Foundry key')
    require(provider['metadata'].get('labels', {}).get('inference.llm-d.ai/ipp-managed') == 'true', 'Provider key is visible to the inference payload processor')
    require('controller.devfile.io/mount-to-devworkspace' not in provider['metadata'].get('labels', {}), 'Provider key is not marked for workspace injection')
    completion()
    secret = resource('secret', 'ai-provider-openai-api-key', 'openshift-operators')['data']
    endpoint = base64.b64decode(secret['OPENAI_BASE_URL']).decode().rstrip('/')
    invalid = urllib.request.Request(endpoint + '/chat/completions', data=json.dumps({
        'model': 'deepseekv4pro', 'messages': [{'role': 'user', 'content': 'Hello'}], 'max_completion_tokens': 16,
    }).encode(), headers={'Content-Type': 'application/json', 'Authorization': 'Bearer invalid-workshop-key'})
    try:
        with urllib.request.urlopen(invalid, timeout=30):
            raise AssertionError('MaaS accepted an invalid API key')
    except urllib.error.HTTPError as error:
        require(error.code in (401, 403), 'MaaS rejects an invalid API key')


def speech():
    dsc = resource('dsc', 'default-dsc')
    require(any(c['type'] == 'Ready' and c['status'] == 'True' for c in dsc['status']['conditions']), 'OpenShift AI Ready')
    model = resource('inferenceservice', 'whisper', 'speech-serving')
    require(any(c['type'] == 'Ready' and c['status'] == 'True' for c in model['status']['conditions']), 'Whisper InferenceService Ready')
    runtime = resource('servingruntime', 'whisper-vllm', 'speech-serving')
    require('--model=RedHatAI/whisper-large-v3-turbo-quantized.w8a8' in runtime['spec']['containers'][0]['args'], 'Whisper uses the RedHatAI INT8 model')
    predictor = model['spec']['predictor']
    require(predictor['model']['resources']['limits']['nvidia.com/gpu'] == '1', 'Whisper requests exactly one GPU')
    nodes = json.loads(oc('get', 'nodes', '-l', 'node.kubernetes.io/instance-type=Standard_NC8as_T4_v3', '-o', 'json'))['items']
    require(len(nodes) == 1 and nodes[0]['status']['allocatable'].get('nvidia.com/gpu') == '1', 'One physical T4 on the speech VM')
    require(not json.loads(oc('get', 'route', '-n', 'speech-serving', '-o', 'json'))['items'], 'Whisper has no public model Route')
    resource('networkpolicy', 'whisper-clients', 'speech-serving')
    print('PASS: Model ingress restricted to workshop application, Dev Spaces and monitoring')


def developer():
    app = os.environ.get('APP_NAME', 'live-transcript')
    ns = app + '-dev'
    deployment = resource('deployment', app, ns)
    require(deployment.get('status', {}).get('availableReplicas', 0) >= 1, 'Application available')
    application = resource('application', app, ns)
    require(application.get('status', {}).get('sync', {}).get('status') == 'Synced' and application.get('status', {}).get('health', {}).get('status') == 'Healthy', 'Personal Argo CD Application Synced / Healthy')
    runs = json.loads(oc('get', 'pipelineruns', '-n', app + '-build', '-o', 'json'))['items']
    require(bool(runs), 'Real Tekton PipelineRuns exist')
    latest = max(runs, key=lambda r: r['metadata']['creationTimestamp'])
    require(any(c['type'] == 'Succeeded' and c['status'] == 'True' for c in latest.get('status', {}).get('conditions', [])), 'Latest Tekton build Succeeded')
    route = resource('route', app, ns)
    require(route['spec'].get('tls', {}).get('termination') == 'edge' and route['spec']['tls'].get('insecureEdgeTerminationPolicy') == 'Redirect', 'HTTPS edge Route redirects HTTP')
    url = 'https://' + route['spec']['host']
    with urllib.request.urlopen(url + '/healthz', timeout=30) as response:
        require(json.load(response) == {'status': 'ok'}, 'Public HTTPS health endpoint')
    with urllib.request.urlopen(url, timeout=30) as response:
        page = response.read().decode('utf-8')
    if os.environ.get('EXPECT_UPDATED_APP') == 'true':
        require('Copy transcript' in html.unescape(page), 'Shipped update copies the transcript')
    spec = deployment['spec']['template']['spec']
    container = spec['containers'][0]
    expected_image = f'image-registry.openshift-image-registry.svc:5000/{app}-build/{app}:latest'
    require(container.get('image') == expected_image, 'Deployment uses the app image from its build namespace')
    require(container.get('imagePullPolicy') == 'Always', 'Mutable latest tag is always pulled')
    requested_image = next((p['value'] for p in latest['spec'].get('params', []) if p['name'] == 'image'), expected_image)
    require(requested_image == container['image'], 'Successful Tekton target matches the deployed image')
    resources = container.get('resources', {})
    require(resources.get('requests') == {'cpu': '50m', 'memory': '64Mi'} and resources.get('limits') == {'cpu': '200m', 'memory': '128Mi'}, 'Resource requests and limits match AGENTS.md')
    require(all(container.get(probe, {}).get('httpGet', {}).get('path') == '/healthz' and container[probe]['httpGet'].get('port') == 8080 for probe in ['livenessProbe', 'readinessProbe']), 'Liveness and readiness probes use /healthz:8080')
    security = spec.get('securityContext', {})
    require(security.get('runAsNonRoot') is True and security.get('seccompProfile', {}).get('type') == 'RuntimeDefault' and 'runAsUser' not in security, 'Deployment uses portable restricted pod security')
    cs = container.get('securityContext', {})
    require(cs.get('allowPrivilegeEscalation') is False and 'ALL' in cs.get('capabilities', {}).get('drop', []), 'Container drops all capabilities and privilege escalation')
    pods = json.loads(oc('get', 'pods', '-n', ns, '-l', 'app.kubernetes.io/name=' + app, '-o', 'json'))['items']
    require(any(c.get('ready') and '@sha256:' in c.get('imageID', '') for p in pods for c in p.get('status', {}).get('containerStatuses', [])), 'Running application pulled a built image digest')
    workspace_ns, workspace_pod = os.environ.get('E2E_NS'), os.environ.get('DEV_POD')
    if workspace_ns or workspace_pod:
        require(bool(workspace_ns and workspace_pod), 'Both E2E_NS and DEV_POD supplied')
        prefix = ['exec', workspace_pod, '-n', workspace_ns, '-c', os.environ.get('DEV_CONTAINER', 'dev'), '--', 'bash', '-c']
        oc(*prefix, 'cd "$PROJECT_SOURCE" && test -f main.go && test -f go.mod && test -f Dockerfile && CGO_ENABLED=0 go build -buildvcs=false -o /dev/null .')
        print('PASS: Generated source files exist and Go build succeeds in the real workspace')
        head = oc(*prefix, 'cd "$PROJECT_SOURCE" && git rev-parse HEAD')
        remote = oc(*prefix, 'cd "$PROJECT_SOURCE" && git ls-remote origin refs/heads/main').split()[0]
        require(head == remote == application['status']['sync'].get('revision'), 'Workspace commit, pushed main and Argo CD revision match')
        oc(*prefix, 'cd "$PROJECT_SOURCE" && git diff --exit-code && git cat-file -e HEAD:main.go')
        print('PASS: Application source committed with no unstaged changes')
    print('Application:', url)
    print('Git revision:', application['status']['sync'].get('revision', 'unknown'))


if __name__ == '__main__':
    try:
        {'platform': platform, 'developer': developer, 'usage': completion, 'speech': speech}[sys.argv[1]]()
    except AssertionError as error:
        sys.exit('FAIL: ' + str(error))
    except urllib.error.HTTPError as error:
        if error.code == 429:
            sys.exit('FAIL: HTTP 429. Check the MaaS subscription allowance and Foundry tokens/minute quota; allow Azure quota changes to propagate before retrying.')
        sys.exit('FAIL: HTTP ' + str(error.code) + '. Check model access or the public application endpoint.')
    except Exception as error:
        # HTTP bodies, subprocess output and Secret contents are deliberately omitted.
        sys.exit('FAIL: ' + type(error).__name__ + '. Check the selected cluster and prerequisite resources.')
