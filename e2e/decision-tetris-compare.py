#!/usr/bin/env python3
"""Compare the deployed local model and heuristic on identical seven-bag seeds.

Requires an authenticated oc session. Only sanitized game results are saved.
Stops on an inference error: there is no retry, fallback or discarded game.
"""
import argparse
import http.cookiejar
import json
import math
from pathlib import Path
import statistics
import subprocess
import time
import urllib.error
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        # Do not forward the short-lived ServiceAccount bearer to another origin.
        return None


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--url', required=True)
    parser.add_argument('--namespace', default='live-transcript-dev')
    parser.add_argument('--service-account', default='workshop-developer')
    parser.add_argument('--seeds', nargs='+', type=int, default=[42, 7, 2026])
    parser.add_argument('--turns', type=int, default=200)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if args.turns < 1:
        parser.error('--turns must be positive')
    base = args.url.rstrip('/')
    opener = urllib.request.build_opener(NoRedirect(),
                urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    report = {'scope': 'Live deployed application; local T4 model versus fixed heuristic',
              'turn_cap': args.turns, 'seeds': args.seeds, 'games': [],
              'limitations': ['This seeded sample does not establish general model performance.',
                              'Scores at the turn cap do not measure full-game survival.',
                              'Both policies receive exact placement outcome features.',
                              'Only model inference latency is instrumented; other decision latencies are null.',
                              'HTTP latency measures the complete request, including network/OAuth overhead.']}

    def save():
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(report, indent=2) + '\n')

    for seed in args.seeds:
        for mode in ('heuristic', 'model'):
            token = subprocess.check_output(['oc', 'create', 'token', args.service_account,
                     '-n', args.namespace, '--duration=10m'], text=True).strip()

            def request(path, body):
                req = urllib.request.Request(base + path, data=json.dumps(body).encode(), headers={
                    'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json', 'Origin': base})
                try:
                    with opener.open(req, timeout=50) as response:
                        return json.load(response)
                except urllib.error.HTTPError as error:
                    raise RuntimeError('Game request failed: HTTP ' + str(error.code)) from None

            state = request('/api/game/new', {'seed': seed})
            turns = []
            while state['game']['turn'] < args.turns and not state['game']['over']:
                piece = state['game']['piece']
                started = time.perf_counter()
                state = request('/api/game/step', {'id': state['id'], 'turn': state['game']['turn'],
                                                 'mode': mode, 'move': ''})
                elapsed = (time.perf_counter() - started) * 1000
                last = state['last']
                if last['source'] not in (mode, 'forced'):
                    raise RuntimeError('Unexpected decision source; refusing a fallback result')
                turns.append({'turn': state['game']['turn'], 'piece': piece, 'choice': last['choice'],
                              'source': last['source'],
                              'latency_ms': last['latency_ms'] if last['source'] == 'model' else None,
                              'http_ms': round(elapsed, 3), 'lines': state['game']['lines'],
                              'score': state['game']['score'], 'trace_id': last.get('trace_id')})
            latency = sorted(t['latency_ms'] for t in turns if t['latency_ms'] is not None)
            game = {'seed': seed, 'mode': mode, 'turns': len(turns), 'lines': state['game']['lines'],
                    'score': state['game']['score'], 'game_over': state['game']['over'],
                    'median_decision_ms': statistics.median(latency) if latency else None,
                    'p95_decision_ms': latency[math.ceil(.95 * len(latency)) - 1] if latency else None,
                    'median_http_ms': round(statistics.median(t['http_ms'] for t in turns), 3),
                    'turn_results': turns}
            report['games'].append(game)
            save()
            print(json.dumps({k: v for k, v in game.items() if k != 'turn_results'}), flush=True)
        a, b = report['games'][-2:]
        n = min(a['turns'], b['turns'])
        if [t['piece'] for t in a['turn_results'][:n]] != [t['piece'] for t in b['turn_results'][:n]]:
            raise RuntimeError('Piece sequences differ; comparison invalid')
    report['completed'] = True
    save()


if __name__ == '__main__':
    main()
