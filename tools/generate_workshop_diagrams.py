#!/usr/bin/env python3
"""Workshop schematic PNGs using Red Hat Display and Red Hat Text.

Typography follows Red Hat brand standards:
https://www.redhat.com/en/about/brand/standards/typography

Digital: Display at 18 px and above, Text below 18 px.
These diagrams use large type (Display throughout) so they stay
readable when Antora scales the PNG down on the page.
"""

from __future__ import annotations

import math
import urllib.request
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

ROOT = Path(__file__).resolve().parents[1]
FONT_DIR = Path(__file__).resolve().parent / ".fonts"
OUT_DIR = ROOT / "content/modules/ROOT/assets/images"

RED = (238, 0, 0)  # #EE0000
BLACK = (21, 21, 21)  # #151515
GRAY = (106, 110, 115)  # #6A6E73
LINE = (210, 210, 210)  # #D2D2D2
BAND = (245, 245, 245)
WHITE = (255, 255, 255)
CANVAS = (255, 255, 255)

# Sized for Antora (~900 px content column). A 1280 px PNG is shown
# at ~70% so these sizes stay readable on the page.
TITLE = 44
SECTION = 26
CARD = 30
BODY = 22
CAPTION = 20
FOOTER = 20

FONT_URLS = {
    "RedHatDisplay-Medium.ttf": (
        "https://raw.githubusercontent.com/RedHatOfficial/RedHatFont/master/"
        "fonts/Proportional/RedHatDisplay/ttf/RedHatDisplay-Medium.ttf"
    ),
    "RedHatDisplay-Regular.ttf": (
        "https://raw.githubusercontent.com/RedHatOfficial/RedHatFont/master/"
        "fonts/Proportional/RedHatDisplay/ttf/RedHatDisplay-Regular.ttf"
    ),
    "RedHatText-Regular.ttf": (
        "https://raw.githubusercontent.com/RedHatOfficial/RedHatFont/master/"
        "fonts/Proportional/RedHatText/ttf/RedHatText-Regular.ttf"
    ),
}

_FONTS: dict[tuple[str, int], ImageFont.FreeTypeFont] = {}


def ensure_fonts() -> None:
    FONT_DIR.mkdir(parents=True, exist_ok=True)
    for name, url in FONT_URLS.items():
        path = FONT_DIR / name
        if path.exists() and path.stat().st_size > 1000:
            continue
        urllib.request.urlretrieve(url, path)


def font(name: str, size: int) -> ImageFont.FreeTypeFont:
    key = (name, size)
    if key not in _FONTS:
        _FONTS[key] = ImageFont.truetype(str(FONT_DIR / name), size)
    return _FONTS[key]


class Board:
    def __init__(self, w: int, h: int):
        self.w = w
        self.h = h
        self.scale = 2
        self.img = Image.new("RGB", (w * self.scale, h * self.scale), CANVAS)
        self.draw = ImageDraw.Draw(self.img)

    def s(self, v: float) -> int:
        return int(round(v * self.scale))

    def display(self, size: int) -> ImageFont.FreeTypeFont:
        return font("RedHatDisplay-Medium.ttf", int(size * self.scale))

    def body_font(self, size: int) -> ImageFont.FreeTypeFont:
        # Body in these diagrams is >= 18 px, so Display is the brand choice.
        if size >= 18:
            return self.display(size)
        return font("RedHatText-Regular.ttf", int(size * self.scale))

    def width(self, s: str, fnt: ImageFont.FreeTypeFont) -> float:
        box = self.draw.textbbox((0, 0), s, font=fnt)
        return (box[2] - box[0]) / self.scale

    def line_h(self, fnt: ImageFont.FreeTypeFont) -> float:
        ascent, descent = fnt.getmetrics()
        return (ascent + descent) / self.scale

    def save(self, path: Path) -> None:
        out = self.img.resize((self.w, self.h), Image.Resampling.LANCZOS)
        path.parent.mkdir(parents=True, exist_ok=True)
        out.save(path, "PNG", optimize=True)
        print(f"wrote {path.relative_to(ROOT)} ({self.w}x{self.h})")

    def frame(self) -> None:
        self.draw.rectangle([0, 0, self.s(10), self.img.height], fill=RED)

    def text(
        self,
        xy: tuple[float, float],
        s: str,
        fnt: ImageFont.FreeTypeFont,
        fill=BLACK,
        anchor: str = "lt",
    ) -> None:
        self.draw.text((self.s(xy[0]), self.s(xy[1])), s, font=fnt, fill=fill, anchor=anchor)

    def line(self, a, b, fill=BLACK, width: float = 2.25) -> None:
        self.draw.line(
            [self.s(a[0]), self.s(a[1]), self.s(b[0]), self.s(b[1])],
            fill=fill,
            width=max(1, self.s(width)),
        )

    def dashed(self, a, b, fill=GRAY, width: float = 2, dash: float = 10, gap: float = 7) -> None:
        x1, y1 = a
        x2, y2 = b
        dx, dy = x2 - x1, y2 - y1
        length = math.hypot(dx, dy) or 1
        ux, uy = dx / length, dy / length
        pos = 0.0
        on = True
        while pos < length:
            step = dash if on else gap
            nxt = min(pos + step, length)
            if on:
                self.line(
                    (x1 + ux * pos, y1 + uy * pos),
                    (x1 + ux * nxt, y1 + uy * nxt),
                    fill=fill,
                    width=width,
                )
            pos = nxt
            on = not on

    def arrow(self, a, b, fill=BLACK, width: float = 2.25, head: float = 14) -> None:
        x1, y1 = a
        x2, y2 = b
        dx, dy = x2 - x1, y2 - y1
        length = math.hypot(dx, dy) or 1
        ux, uy = dx / length, dy / length
        tip = (x2, y2)
        base = (x2 - ux * head, y2 - uy * head)
        self.line(a, base, fill=fill, width=width)
        px, py = -uy, ux
        p1 = (base[0] + px * head * 0.42, base[1] + py * head * 0.42)
        p2 = (base[0] - px * head * 0.42, base[1] - py * head * 0.42)
        self.draw.polygon(
            [
                self.s(tip[0]),
                self.s(tip[1]),
                self.s(p1[0]),
                self.s(p1[1]),
                self.s(p2[0]),
                self.s(p2[1]),
            ],
            fill=fill,
        )

    def rounded(self, box, radius: float, fill=WHITE, outline=LINE, width: float = 1.5) -> None:
        x0, y0, x1, y1 = box
        self.draw.rounded_rectangle(
            [self.s(x0), self.s(y0), self.s(x1), self.s(y1)],
            radius=self.s(radius),
            fill=fill,
            outline=outline,
            width=max(1, self.s(width)),
        )

    def accent_bar(self, box, width: float = 7) -> None:
        x0, y0, _x1, y1 = box
        self.draw.rectangle(
            [self.s(x0), self.s(y0 + 8), self.s(x0 + width), self.s(y1 - 8)],
            fill=RED,
        )

    def wrap(self, s: str, fnt: ImageFont.FreeTypeFont, max_w: float) -> list[str]:
        words = s.split()
        lines: list[str] = []
        cur = ""
        for word in words:
            trial = f"{cur} {word}".strip()
            if self.width(trial, fnt) <= max_w:
                cur = trial
            else:
                if cur:
                    lines.append(cur)
                cur = word
        if cur:
            lines.append(cur)
        return lines or [s]

    def card(
        self,
        box,
        title: str,
        subtitle: str,
        pad: float = 28,
        title_size: int = CARD,
        body_size: int = BODY,
    ) -> None:
        self.rounded(box, radius=8, fill=WHITE, outline=LINE, width=1.5)
        self.accent_bar(box)
        x0, y0, x1, y1 = box
        tx = x0 + pad + 8
        max_w = (x1 - tx) - pad
        title_f = self.display(title_size)
        sub_f = self.body_font(body_size)
        title_lines = self.wrap(title, title_f, max_w)
        sub_lines = self.wrap(subtitle, sub_f, max_w)
        th = self.line_h(title_f)
        sh = self.line_h(sub_f)
        block = len(title_lines) * (th + 4) + 10 + len(sub_lines) * (sh + 4)
        y = y0 + max(pad, ((y1 - y0) - block) / 2)
        for line in title_lines:
            self.text((tx, y), line, title_f, fill=BLACK)
            y += th + 4
        y += 8
        for line in sub_lines:
            self.text((tx, y), line, sub_f, fill=GRAY)
            y += sh + 4
        if y > y1 - 8:
            raise SystemExit(f"card overflow: {title!r} / {subtitle!r} bottom={y:.0f} box={y1:.0f}")

    def band(self, box) -> None:
        self.rounded(box, radius=12, fill=BAND, outline=BAND, width=0)

    def footer(self, caption: str) -> None:
        y = self.h - 72
        self.line((56, y), (self.w - 56, y), fill=RED, width=2.5)
        self.text((56, y + 22), "Red Hat", self.display(FOOTER), fill=BLACK)
        self.text((self.w - 56, y + 22), caption, self.body_font(FOOTER), fill=GRAY, anchor="rt")

    def title(self, s: str, y: float = 40) -> None:
        self.text((56, y), s, self.display(TITLE), fill=BLACK)

    def caption_stack(self, cx: float, cy: float, lines: list[str]) -> None:
        """Centered multi-line arrow label so it fits in a narrow gap."""
        fnt = self.body_font(CAPTION)
        h = self.line_h(fnt)
        total = len(lines) * h + (len(lines) - 1) * 2
        y = cy - total / 2
        for line in lines:
            self.text((cx, y), line, fnt, fill=GRAY, anchor="mt")
            y += h + 2

    def columns(self, n: int, y0: float, y1: float, left: float = 48, right: float | None = None, gap: float = 24):
        right = (self.w - 48) if right is None else right
        cw = ((right - left) - gap * (n - 1)) / n
        return [(left + i * (cw + gap), y0, left + i * (cw + gap) + cw, y1) for i in range(n)]


def architecture() -> None:
    b = Board(1280, 900)
    b.frame()
    b.title("Two model paths on ARO")
    b.band((36, 110, 1244, 410))
    b.text((56, 125), "Coding — governed access to an external model", b.display(SECTION), fill=RED)
    boxes = b.columns(3, 185, 385, left=56, right=1224, gap=36)
    for box, (title, sub) in zip(boxes, [
        ("Dev Spaces + OpenCode", "MaaS key · AGENTS.md"),
        ("OpenShift AI MaaS", "API keys · token quota"),
        ("DeepSeek / Foundry", "External endpoint · Azure key"),
    ]):
        b.card(box, title, sub, title_size=28, body_size=20)
    for left, right in zip(boxes, boxes[1:]):
        b.arrow((left[2], 280), (right[0], 280), head=12)
    b.band((36, 440, 1244, 755))
    b.text((56, 455), "Application — microphone audio stays on ARO", b.display(SECTION), fill=RED)
    boxes = b.columns(3, 520, 725, left=56, right=1224, gap=36)
    for box, (title, sub) in zip(boxes, [
        ("Browser microphone", "Independent short WAV clips"),
        ("Live Transcript", "Go backend · Tekton + Argo CD"),
        ("Whisper INT8", "OpenShift AI + Red Hat vLLM · one T4"),
    ]):
        b.card(box, title, sub)
    for left, right in zip(boxes, boxes[1:]):
        b.arrow((left[2], 615), (right[0], 615), head=12)
    b.footer("Coding goes to Foundry through MaaS. Speech stays inside the cluster.")
    b.save(OUT_DIR / "architecture-overview.png")


def model_serving() -> None:
    b = Board(1280, 720)
    b.frame()
    b.title("Whisper on one T4")
    boxes = b.columns(3, 220, 470, left=56, right=1224, gap=36)
    for box, (title, sub) in zip(boxes, [
        ("Go backend", "POST /api/transcribe · bounded WAV uploads"),
        ("KServe", "Internal service only · network policy"),
        ("Red Hat vLLM", "RedHatAI Whisper Turbo INT8 · one 16 GB T4"),
    ]):
        b.card(box, title, sub)
    for left, right in zip(boxes, boxes[1:]):
        b.arrow((left[2], 340), (right[0], 340), head=12)
    b.text((56, 520), "Browser → HTTPS app → internal /v1/audio/transcriptions", b.display(26))
    b.footer("Near-real-time results from short clips; no public model Route.")
    b.save(OUT_DIR / "model-serving-architecture.png")


def cicd() -> None:
    b = Board(1280, 680)
    b.frame()
    b.title("Prompt to deployment")

    steps = [
        ("1", "Prompt OpenCode", "reads AGENTS.md"),
        ("2", "Ephemeral Git", "personal repo"),
        ("3", "Tekton", "git-clone + buildah"),
        ("4", "Argo CD", "developer instance"),
        ("5", "HTTPS Route", "Live Transcript live"),
    ]
    y0, y1 = 250, 560
    boxes = b.columns(5, y0, y1, left=40, right=1240, gap=20)
    for i, (box, (num, title, sub)) in enumerate(zip(boxes, steps)):
        x0, _, x1, _ = box
        b.rounded(box, radius=8, fill=WHITE, outline=LINE, width=1.5)
        b.accent_bar(box)
        title_f = b.display(CARD)
        sub_f = b.body_font(BODY)
        max_w = (x1 - x0) - 36
        num_f = b.display(32)
        title_lines = b.wrap(title, title_f, max_w)
        sub_lines = b.wrap(sub, sub_f, max_w)
        num_h = b.line_h(num_f)
        th = b.line_h(title_f)
        sh = b.line_h(sub_f)
        block = num_h + 12 + len(title_lines) * (th + 2) + 8 + len(sub_lines) * (sh + 2)
        ty = y0 + (y1 - y0 - block) / 2
        b.text((x0 + 18, ty), num, num_f, fill=RED)
        ty += num_h + 12
        for line in title_lines:
            b.text((x0 + 18, ty), line, title_f, fill=BLACK)
            ty += th + 2
        ty += 6
        for line in sub_lines:
            b.text((x0 + 18, ty), line, sub_f, fill=GRAY)
            ty += sh + 2
        if ty > y1 - 10:
            raise SystemExit(f"cicd overflow: {title!r} bottom={ty:.0f} box={y1}")
        if i:
            prev = boxes[i - 1]
            mid_y = (y0 + y1) / 2
            b.arrow((prev[2], mid_y), (x0, mid_y), head=11)

    mid_x = (boxes[0][0] + boxes[0][2]) / 2
    chip_w = 240
    call = (mid_x - chip_w / 2, 118, mid_x + chip_w / 2, 200)
    b.rounded(call, radius=8, fill=WHITE, outline=LINE, width=1.5)
    b.text((mid_x, 140), "DeepSeek via MaaS", b.display(24), fill=BLACK, anchor="mt")
    b.dashed((mid_x, y0), (mid_x, call[3]), fill=GRAY, width=2)
    b.arrow((mid_x, y0), (mid_x, call[3] + 1), fill=GRAY, width=2, head=10)

    b.footer("Coding on Foundry. Speech inference inside ARO.")
    b.save(OUT_DIR / "cicd-gitops.png")


def main() -> None:
    ensure_fonts()
    architecture()
    model_serving()
    cicd()


if __name__ == "__main__":
    main()
