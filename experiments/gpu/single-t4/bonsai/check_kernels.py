import importlib.util
import json
import time
from pathlib import Path

import torch

spec = importlib.util.spec_from_file_location(
    "t4kernels", str(Path(__file__).with_name("kernels_t4.py"))
)
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
torch.manual_seed(42)
device = "cuda"
n, k = 64, 1024
codes = torch.randint(0, 3, (n, k), dtype=torch.int32, device=device)
packed = torch.zeros((n, k // 16), dtype=torch.int32, device=device)
for j in range(16):
    packed |= codes[:, j::16] << (j * 2)
scales = torch.rand((n, k // 128), dtype=torch.float16, device=device) * 0.05
signs = (torch.randint(0, 2, (k,), device=device, dtype=torch.int32) * 2 - 1).float()
w, s = m.block_layout(packed, scales)
for rows in [1, 4, 17, 256]:
    x = torch.randn((rows, k), device=device, dtype=torch.float16)
    r = x.float() * signs
    for stride in [1 << i for i in range(10)]:
        a = r.reshape(rows, -1, stride * 2)
        lo, hi = a[..., :stride].clone(), a[..., stride:].clone()
        a[..., :stride] = lo + hi
        a[..., stride:] = lo - hi
    r = r / 32
    ref = r @ ((codes.float() - 1) * scales.float().repeat_interleave(128, dim=1)).T
    start = time.monotonic()
    y = m.ternary_linear(x, w, s, signs)
    torch.cuda.synchronize()
    torch.testing.assert_close(y.float(), ref, atol=0.01, rtol=0.03)
    print(
        json.dumps(
            {
                "rows": rows,
                "passed": True,
                "max_error": float((y.float() - ref).abs().max()),
                "seconds_with_compilation": time.monotonic() - start,
            }
        ),
        flush=True,
    )
