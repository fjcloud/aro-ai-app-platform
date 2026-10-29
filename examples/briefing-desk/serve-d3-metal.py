#!/usr/bin/env python3
"""Serve the workshop's pinned d3-mini on Apple Metal, with CPU fallback disabled."""
import os
import platform
import sys

os.environ['PYTORCH_ENABLE_MPS_FALLBACK'] = '0'

if platform.system() != 'Darwin' or platform.machine() != 'arm64':
    raise SystemExit('This launcher requires an Apple Silicon Mac.')

import torch  # noqa: E402
from huggingface_hub import snapshot_download  # noqa: E402

if not torch.backends.mps.is_available():
    raise SystemExit('Metal/MPS is unavailable; CPU fallback is disabled.')

model = snapshot_download(
    'vllm-sr/d3-mini',
    revision='a475367de0d8133a3c1a2a0dc12303d89cfa2ab4',
)
os.execv(sys.executable, [
    sys.executable, '-m', 'vllm_srun.cli', 'serve', model,
    '--served-model-name', 'vllm-sr/d3-mini',
    '--device', 'mps', '--dtype', 'float16',
    '--host', '127.0.0.1', '--port', '18093',
    '--max-num-seqs', '1', '--max-num-batched-tokens', '4096',
    '--media-workers', '0', '--result-cache-entries', '0',
    '--threads', '4', '--load-attempts', '1',
])
