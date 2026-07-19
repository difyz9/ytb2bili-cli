#!/usr/bin/env python3
"""Synthesize one WAV clip per SRT entry through the IndexTTS2 HTTP API."""

from __future__ import annotations

import argparse
import asyncio
import json
import sys
import urllib.error
import urllib.request
from pathlib import Path

import pysrt


def request_json(url: str, payload: dict[str, object] | None, timeout: float) -> dict:
    data = None if payload is None else json.dumps(payload, ensure_ascii=False).encode("utf-8")
    request = urllib.request.Request(
        url,
        data=data,
        headers={"Content-Type": "application/json"},
        method="GET" if payload is None else "POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            body = response.read().decode("utf-8")
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        try:
            detail = json.loads(body).get("error", body)
        except json.JSONDecodeError:
            detail = body
        raise RuntimeError(f"IndexTTS2 HTTP {exc.code}: {detail}") from exc
    result = json.loads(body)
    if not isinstance(result, dict):
        raise RuntimeError(f"unexpected API response: {result!r}")
    if result.get("error"):
        raise RuntimeError(f"IndexTTS2 error: {result['error']}")
    return result


def health_check(api_url: str, timeout: float) -> dict:
    result = request_json(f"{api_url}/health", None, timeout)
    if result.get("status") != "ok" or result.get("model_loaded") is False:
        raise RuntimeError(f"IndexTTS2 is not ready: {result}")
    return result


def synthesize_once(
    api_url: str,
    text: str,
    output: Path,
    server_output: str,
    remote_output: bool,
    emotion: str,
    emo_alpha: float,
    ref_audio: str,
    use_emo_text: bool,
    emo_text: str,
    timeout: float,
) -> None:
    payload: dict[str, object] = {
        "text": text,
        "output": server_output,
        "emotion": emotion,
        "emo_alpha": emo_alpha,
        "use_emo_text": use_emo_text,
    }
    if ref_audio:
        payload["ref_audio"] = ref_audio
    if emo_text:
        payload["emo_text"] = emo_text

    result = request_json(f"{api_url}/synthesize", payload, timeout)
    returned = result.get("output")
    if not returned:
        raise RuntimeError(f"API response has no output path: {result}")
    if not server_output.startswith("/"):
        raise RuntimeError(f"invalid server output path: {returned!r}")
    if not remote_output and (not output.exists() or output.stat().st_size == 0):
        raise RuntimeError(f"API did not create requested output: {output}")


async def synthesize_entry(
    semaphore: asyncio.Semaphore,
    index: int,
    text: str,
    output: Path,
    server_output: str,
    args: argparse.Namespace,
) -> None:
    async with semaphore:
        last_error: Exception | None = None
        for attempt in range(1, args.retries + 1):
            try:
                await asyncio.to_thread(
                    synthesize_once,
                    args.api_url,
                    text,
                    output,
                    server_output,
                    bool(args.server_output_dir),
                    args.emotion,
                    args.emo_alpha,
                    args.ref_audio,
                    args.use_emo_text,
                    args.emo_text,
                    args.timeout,
                )
                return
            except (OSError, RuntimeError, urllib.error.URLError) as exc:
                last_error = exc
                output.unlink(missing_ok=True)
                if attempt < args.retries:
                    await asyncio.sleep(min(2**attempt, 8))
        raise RuntimeError(f"subtitle {index}: {last_error}")


async def run(args: argparse.Namespace) -> dict[str, object]:
    health = await asyncio.to_thread(health_check, args.api_url, args.timeout)
    subtitles = pysrt.open(args.srt, encoding="utf-8")
    output_dir = Path(args.output_dir).resolve()
    output_dir.mkdir(parents=True, exist_ok=True)
    semaphore = asyncio.Semaphore(args.concurrency)
    tasks = []
    skipped = 0

    for position, subtitle in enumerate(subtitles, start=1):
        index = subtitle.index or position
        output = output_dir / f"{index}.wav"
        server_output = (
            f"{args.server_output_dir}/{index}.wav"
            if args.server_output_dir
            else str(output)
        )
        text = " ".join(subtitle.text.replace("\\N", " ").split())
        if not text:
            skipped += 1
            continue
        if output.exists() and output.stat().st_size > 0 and not args.overwrite:
            skipped += 1
            continue
        tasks.append(synthesize_entry(semaphore, index, text, output, server_output, args))

    completed = 0
    for task in asyncio.as_completed(tasks):
        await task
        completed += 1
        if completed % 25 == 0 or completed == len(tasks):
            print(f"TTS progress: {completed}/{len(tasks)}", file=sys.stderr)

    return {
        "provider": "indextts2",
        "api_url": args.api_url,
        "server": health,
        "audio_dir": str(output_dir),
        "server_output_dir": args.server_output_dir or None,
        "generated": completed,
        "skipped": skipped,
        "subtitles": len(subtitles),
        "emotion": args.emotion,
        "emo_alpha": args.emo_alpha,
        "ref_audio": args.ref_audio or None,
    }


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--srt", required=True, help="UTF-8 SRT subtitle file")
    parser.add_argument("--output-dir", required=True, help="Directory for indexed WAV clips")
    parser.add_argument("--api-url", default="http://localhost:18765")
    parser.add_argument(
        "--server-output-dir",
        default="",
        help="Output directory on a remote IndexTTS2 host; files are not copied locally",
    )
    parser.add_argument("--emotion", default="default")
    parser.add_argument("--emo-alpha", type=float, default=0.6)
    parser.add_argument("--ref-audio", default="", help="Reference WAV path visible to the server")
    parser.add_argument("--use-emo-text", action="store_true")
    parser.add_argument("--emo-text", default="")
    parser.add_argument("--concurrency", type=int, default=1)
    parser.add_argument("--retries", type=int, default=3)
    parser.add_argument("--timeout", type=float, default=180.0)
    parser.add_argument("--overwrite", action="store_true")
    args = parser.parse_args()
    if args.concurrency < 1 or args.retries < 1 or args.timeout <= 0:
        parser.error("--concurrency, --retries, and --timeout must be positive")
    if not 0.0 <= args.emo_alpha <= 1.0:
        parser.error("--emo-alpha must be between 0.0 and 1.0")
    args.api_url = args.api_url.rstrip("/")
    args.server_output_dir = args.server_output_dir.rstrip("/")
    return args


def main() -> int:
    args = parse_args()
    try:
        result = asyncio.run(run(args))
    except Exception as exc:
        print(f"TTS synthesis failed: {exc}", file=sys.stderr)
        return 1
    print(json.dumps(result, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
