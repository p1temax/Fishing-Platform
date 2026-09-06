#!/usr/bin/env python3
"""
Local QR screen relay for Fishing Platform.

Continuously captures the screen (or a region), detects QR codes, and uploads
ONLY image frames to POST /api/qr-relay/frames/ using the relay upload token.
Also sends heartbeats so the console can show online/stale health.

Dependencies (macOS may need: brew install zbar):
  pip install -r requirements.txt
"""

from __future__ import annotations

import argparse
import hashlib
import io
import sys
import time
from pathlib import Path
from typing import Any

import requests
import yaml

try:
    import mss
    from PIL import Image
    from pyzbar.pyzbar import decode as zbar_decode
except ImportError as exc:
    print(f"Missing dependency: {exc}", file=sys.stderr)
    print("Run: pip install -r requirements.txt", file=sys.stderr)
    print("On macOS also: brew install zbar", file=sys.stderr)
    sys.exit(1)


def load_config(path: Path) -> dict[str, Any]:
    data = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    if not isinstance(data, dict):
        raise ValueError("config must be a mapping")
    return data


def save_config(path: Path, data: dict[str, Any]) -> None:
    path.write_text(
        yaml.safe_dump(data, default_flow_style=False, allow_unicode=True, sort_keys=False),
        encoding="utf-8",
    )


def grab_frame(region: dict[str, int] | None) -> Image.Image:
    with mss.mss() as sct:
        if region:
            monitor = {
                "left": int(region["left"]),
                "top": int(region["top"]),
                "width": int(region["width"]),
                "height": int(region["height"]),
            }
        else:
            monitor = sct.monitors[1]
        shot = sct.grab(monitor)
        return Image.frombytes("RGB", shot.size, shot.bgra, "raw", "BGRX")


def decode_payloads(image: Image.Image) -> list[str]:
    results = zbar_decode(image)
    out: list[str] = []
    for item in results:
        try:
            text = item.data.decode("utf-8", errors="replace").strip()
        except Exception:
            continue
        if text:
            out.append(text)
    return out


def image_fingerprint(png_bytes: bytes, payload: str) -> str:
    h = hashlib.sha256()
    h.update(payload.encode("utf-8"))
    h.update(b"|")
    h.update(png_bytes)
    return h.hexdigest()


def to_png_bytes(image: Image.Image) -> bytes:
    buf = io.BytesIO()
    image.save(buf, format="PNG", optimize=True)
    return buf.getvalue()


def upload_frame(
    platform_url: str,
    token: str,
    png_bytes: bytes,
    payload: str,
    verify_tls: bool,
) -> None:
    url = platform_url.rstrip("/") + "/api/qr-relay/frames/"
    files = {"image": ("qr.png", png_bytes, "image/png")}
    data = {}
    if payload:
        data["payload"] = payload
    headers = {"X-QR-Relay-Token": token}
    resp = requests.post(
        url,
        headers=headers,
        files=files,
        data=data,
        timeout=30,
        verify=verify_tls,
    )
    if resp.status_code >= 400:
        raise RuntimeError(f"upload failed HTTP {resp.status_code}: {resp.text[:300]}")


def send_heartbeat(platform_url: str, token: str, verify_tls: bool) -> None:
    url = platform_url.rstrip("/") + "/api/qr-relay/heartbeat/"
    headers = {"X-QR-Relay-Token": token}
    resp = requests.post(url, headers=headers, timeout=15, verify=verify_tls)
    if resp.status_code >= 400:
        raise RuntimeError(f"heartbeat failed HTTP {resp.status_code}: {resp.text[:300]}")


def pick_region_gui() -> dict[str, int]:
    """Fullscreen drag-to-select ROI using tkinter (stdlib)."""
    try:
        import tkinter as tk
    except ImportError as exc:
        raise RuntimeError("tkinter is required for --pick-region") from exc

    root = tk.Tk()
    root.title("Select QR capture region")
    root.attributes("-fullscreen", True)
    root.attributes("-topmost", True)
    try:
        root.attributes("-alpha", 0.35)
    except tk.TclError:
        pass
    root.configure(bg="black")
    canvas = tk.Canvas(root, cursor="cross", bg="black", highlightthickness=0)
    canvas.pack(fill=tk.BOTH, expand=True)

    state: dict[str, Any] = {"x0": 0, "y0": 0, "rect": None, "done": None}

    def on_press(event: Any) -> None:
        state["x0"], state["y0"] = event.x, event.y
        if state["rect"] is not None:
            canvas.delete(state["rect"])
        state["rect"] = canvas.create_rectangle(
            event.x, event.y, event.x, event.y, outline="#22c55e", width=2
        )

    def on_drag(event: Any) -> None:
        if state["rect"] is None:
            return
        canvas.coords(state["rect"], state["x0"], state["y0"], event.x, event.y)

    def on_release(event: Any) -> None:
        x0, y0 = int(state["x0"]), int(state["y0"])
        x1, y1 = int(event.x), int(event.y)
        left, top = min(x0, x1), min(y0, y1)
        width, height = abs(x1 - x0), abs(y1 - y0)
        if width < 8 or height < 8:
            return
        state["done"] = {"left": left, "top": top, "width": width, "height": height}
        root.destroy()

    def on_escape(_: Any) -> None:
        state["done"] = None
        root.destroy()

    canvas.bind("<ButtonPress-1>", on_press)
    canvas.bind("<B1-Motion>", on_drag)
    canvas.bind("<ButtonRelease-1>", on_release)
    root.bind("<Escape>", on_escape)
    hint = tk.Label(
        root,
        text="Drag to select QR region · Esc to cancel",
        fg="white",
        bg="#111827",
        font=("Helvetica", 16),
    )
    hint.place(relx=0.5, y=24, anchor="n")
    root.mainloop()
    if not state["done"]:
        raise RuntimeError("region selection cancelled")
    return state["done"]


def main() -> int:
    parser = argparse.ArgumentParser(description="Fishing Platform QR screen relay")
    parser.add_argument(
        "-c",
        "--config",
        default=str(Path(__file__).with_name("config.yaml")),
        help="path to config.yaml",
    )
    parser.add_argument(
        "--pick-region",
        action="store_true",
        help="open a fullscreen ROI picker and write region into config.yaml",
    )
    args = parser.parse_args()
    cfg_path = Path(args.config)
    if not cfg_path.exists():
        example = Path(__file__).with_name("config.example.yaml")
        print(f"Config not found: {cfg_path}", file=sys.stderr)
        print(f"Copy {example} to config.yaml and edit tokens.", file=sys.stderr)
        return 1

    cfg = load_config(cfg_path)

    if args.pick_region:
        region = pick_region_gui()
        cfg["region"] = region
        save_config(cfg_path, cfg)
        print(f"[qr-relay] saved region to {cfg_path}: {region}")
        return 0

    platform_url = str(cfg.get("platform_url") or "").strip()
    token = str(cfg.get("upload_token") or "").strip()
    interval_ms = int(cfg.get("interval_ms") or 500)
    heartbeat_ms = int(cfg.get("heartbeat_interval_ms") or 3000)
    dedupe = bool(cfg.get("dedupe", True))
    verify_tls = bool(cfg.get("verify_tls", True))
    region = cfg.get("region")
    if region is not None and not isinstance(region, dict):
        print("region must be a mapping or omitted", file=sys.stderr)
        return 1

    if not platform_url or not token:
        print("platform_url and upload_token are required", file=sys.stderr)
        return 1

    print(
        f"[qr-relay] platform={platform_url} interval={interval_ms}ms "
        f"heartbeat={heartbeat_ms}ms"
    )
    last_fp = ""
    last_heartbeat = 0.0
    failures = 0
    while True:
        try:
            now = time.time()
            frame = grab_frame(region if isinstance(region, dict) else None)
            payloads = decode_payloads(frame)
            if not payloads:
                if heartbeat_ms > 0 and (now - last_heartbeat) * 1000 >= heartbeat_ms:
                    send_heartbeat(platform_url, token, verify_tls)
                    last_heartbeat = now
                    print("[qr-relay] heartbeat (no QR in frame)")
                time.sleep(max(interval_ms, 100) / 1000.0)
                continue
            payload = payloads[0]
            png_bytes = to_png_bytes(frame)
            fp = image_fingerprint(png_bytes, payload)
            if dedupe and fp == last_fp:
                if heartbeat_ms > 0 and (now - last_heartbeat) * 1000 >= heartbeat_ms:
                    send_heartbeat(platform_url, token, verify_tls)
                    last_heartbeat = now
                time.sleep(max(interval_ms, 100) / 1000.0)
                continue
            upload_frame(platform_url, token, png_bytes, payload, verify_tls)
            last_fp = fp
            last_heartbeat = now
            failures = 0
            print(f"[qr-relay] uploaded ({len(png_bytes)} bytes) payload={payload[:80]}")
        except KeyboardInterrupt:
            print("\n[qr-relay] stopped")
            return 0
        except Exception as exc:  # noqa: BLE001 — keep loop alive
            failures += 1
            print(f"[qr-relay] error: {exc}", file=sys.stderr)
            time.sleep(min(5.0, 0.5 * failures))
            continue
        time.sleep(max(interval_ms, 100) / 1000.0)


if __name__ == "__main__":
    raise SystemExit(main())
