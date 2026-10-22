"""Experimental SM75 backend for the fraserprice prism_ternary vLLM plugin.

Keeps original 2-bit codes, scales and signed Hadamard rotation. FP16 cuBLAS
matmul for prefill, FP32 packed GEMV for decode. No dense model-weight cache.
"""

import torch
import triton
import triton.language as tl

GROUP = 128
HADAMARD_BLOCK = 1024
LANES = 16
ROW_BLOCK = 32


def block_layout(weight, scales):
    out, words = weight.shape
    groups = scales.shape[1]
    return (
        weight.view(out // 32, 32, groups, 8)
        .permute(0, 2, 1, 3)
        .contiguous()
        .view(out, words),
        scales.view(out // 32, 32, groups)
        .permute(0, 2, 1)
        .contiguous()
        .view(out, groups),
    )


@triton.jit
def _rotate(X, S, Y, K: tl.constexpr, B: tl.constexpr):
    row = tl.program_id(0)
    col = tl.program_id(1) * B + tl.arange(0, B)
    idx = tl.arange(0, B)
    value = tl.load(X + row * K + col).to(tl.float32) * tl.load(S + col)
    for stage in tl.static_range(10):
        stride = 1 << stage
        other = tl.gather(value, idx ^ stride, 0)
        value = tl.where((idx & stride) == 0, value + other, other - value)
    tl.store(Y + row * K + col, value * 0.03125)


@triton.jit
def _gemv(
    X, W, S, Y, N: tl.constexpr, K: tl.constexpr, BN: tl.constexpr, BK: tl.constexpr
):
    row = tl.program_id(0) * BN + tl.arange(0, BN)
    m = tl.program_id(1)
    kk = tl.arange(0, BK)
    acc = tl.full((BN, BK), 0, tl.float32)
    for base in range(tl.cdiv(K, BK)):
        k = base * BK + kk
        block = (row[:, None] // 32 * (K // 128) + k[None, :] // 128) * 32 + row[
            :, None
        ] % 32
        word = tl.load(
            W + block * 8 + k[None, :] % 128 // 16,
            (row[:, None] < N) & (k[None, :] < K),
            other=0,
        )
        scale = tl.load(S + block, (row[:, None] < N) & (k[None, :] < K), other=0).to(
            tl.float32
        )
        weight = (((word >> (2 * (k[None, :] % 16))) & 3) - 1).to(tl.float32) * scale
        x = tl.load(X + m * K + k, k < K, other=0).to(tl.float32)
        acc = acc + weight * x[None, :]
    tl.store(Y + m * N + row, tl.sum(acc, 1), row < N)


@triton.jit
def _dequant(
    W, S, Y, START: tl.constexpr, ROWS: tl.constexpr, K: tl.constexpr, B: tl.constexpr
):
    index = tl.program_id(0) * B + tl.arange(0, B)
    row = START + index // K
    col = index % K
    mask = index < ROWS * K
    block = (row // 32 * (K // 128) + col // 128) * 32 + row % 32
    word = tl.load(W + block * 8 + col % 128 // 16, mask, other=0)
    scale = tl.load(S + block, mask, other=0).to(tl.float32)
    value = (((word >> (2 * (col % 16))) & 3) - 1).to(tl.float32) * scale
    tl.store(Y + index, value, mask)


@torch.library.custom_op("prism_ternary::linear", mutates_args=())
def ternary_linear(
    x: torch.Tensor, weight: torch.Tensor, scales: torch.Tensor, signs: torch.Tensor
) -> torch.Tensor:
    original = x.shape
    x = x.reshape(-1, x.shape[-1]).contiguous()
    m, k = x.shape
    n = weight.shape[0]
    rotated = torch.empty((m, k), dtype=torch.float32, device=x.device)
    y = torch.empty((m, n), dtype=x.dtype, device=x.device)
    _rotate[(m, k // 1024)](x, signs, rotated, k, 1024)
    if m <= 4:
        _gemv[(triton.cdiv(n, 8), m)](
            rotated, weight, scales, y, n, k, BN=8, BK=512, num_warps=4
        )
    else:
        # Triton lowers dot to SIMT on SM75. cuBLAS uses the T4 FP16 Tensor Cores.
        # Dequantize one output block temporarily; never retain dense model weights.
        half_rotated = rotated.to(torch.float16)
        for start in range(0, n, 8192):
            rows = min(8192, n - start)
            dense = torch.empty((rows, k), dtype=torch.float16, device=x.device)
            _dequant[(triton.cdiv(rows * k, 512),)](
                weight, scales, dense, start, rows, k, B=512
            )
            y[:, start : start + rows] = half_rotated @ dense.T
    return y.reshape(*original[:-1], n)


@ternary_linear.register_fake
def _fake(x, weight, scales, signs):
    return x.new_empty(*x.shape[:-1], weight.shape[0])
