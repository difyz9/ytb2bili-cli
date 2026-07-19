#!/usr/bin/env python3
"""Align per-subtitle audio clips to an SRT timeline and replace a video track."""

import argparse
import json
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path


PATTERNS = ("{index}.mp3", "{index}.wav", "audio_{index}.mp3", "audio_{index}.wav", "{index:03d}.mp3", "{index:03d}.wav")


def require_dependencies():
    missing = []
    for executable in ("ffmpeg", "ffprobe"):
        if not shutil.which(executable):
            missing.append(executable)
    try:
        from pydub import AudioSegment  # noqa: F401
        import pysrt  # noqa: F401
    except ImportError as exc:
        missing.append(str(exc))
    if missing:
        raise RuntimeError("missing dependencies: " + ", ".join(missing))


def find_audio(audio_dir: Path, index: int):
    for pattern in PATTERNS:
        candidate = audio_dir / pattern.format(index=index)
        if candidate.is_file():
            return candidate
    return None


def video_duration(path: Path) -> float:
    result = subprocess.run(
        ["ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=nw=1:nk=1", str(path)],
        check=True, capture_output=True, text=True,
    )
    return float(result.stdout.strip())


def atempo_filter(factor: float) -> str:
    factors = []
    while factor > 2.0:
        factors.append(2.0)
        factor /= 2.0
    while factor < 0.5:
        factors.append(0.5)
        factor /= 0.5
    factors.append(factor)
    return ",".join(f"atempo={value:.6f}" for value in factors)


def adjust_speed(source: Path, output: Path, factor: float):
    subprocess.run(
        ["ffmpeg", "-y", "-v", "error", "-i", str(source), "-filter:a", atempo_filter(factor), str(output)],
        check=True,
    )


def synchronize(video: Path, srt_path: Path, audio_dir: Path, output: Path, speed_adjust: bool, missing_mode: str, max_speed: float):
    from pydub import AudioSegment
    import pysrt

    subtitles = pysrt.open(str(srt_path), encoding="utf-8")
    if not subtitles:
        raise ValueError("subtitle file is empty")
    video_ms = int(video_duration(video) * 1000)
    duration_ms = max(video_ms, subtitles[-1].end.ordinal) + 60000
    timeline = AudioSegment.silent(duration=duration_ms, frame_rate=44100)
    stats = {"clips": 0, "adjusted": 0, "missing": 0, "delayed": 0, "max_delay_ms": 0}
    cursor_ms = 0

    with tempfile.TemporaryDirectory(prefix="ytb2bili-audio-sync-") as temp:
        temp_dir = Path(temp)
        for subtitle in subtitles:
            source = find_audio(audio_dir, subtitle.index)
            if source is None:
                stats["missing"] += 1
                if missing_mode == "error":
                    raise FileNotFoundError(f"missing audio for subtitle index {subtitle.index}")
                continue
            clip = AudioSegment.from_file(source)
            target_ms = max(1, subtitle.end.ordinal - subtitle.start.ordinal)
            ratio = len(clip) / target_ms
            if speed_adjust and ratio > 1.15:
                adjusted = temp_dir / f"{subtitle.index}.wav"
                adjust_speed(source, adjusted, min(max_speed, ratio))
                clip = AudioSegment.from_file(adjusted)
                stats["adjusted"] += 1
            # Preserve natural speech: if a prior sentence runs long, consume
            # following silence instead of overlapping or truncating speech.
            position_ms = max(subtitle.start.ordinal, cursor_ms)
            delay_ms = position_ms - subtitle.start.ordinal
            if delay_ms > 0:
                stats["delayed"] += 1
                stats["max_delay_ms"] = max(stats["max_delay_ms"], delay_ms)
            timeline = timeline.overlay(clip, position=position_ms)
            cursor_ms = position_ms + len(clip)
            stats["clips"] += 1

        synced_audio = temp_dir / "synced_audio.wav"
        timeline.export(synced_audio, format="wav")
        output.parent.mkdir(parents=True, exist_ok=True)
        subprocess.run([
            "ffmpeg", "-y", "-v", "error", "-i", str(video), "-i", str(synced_audio),
            "-map", "0:v:0", "-map", "1:a:0", "-c:v", "copy", "-c:a", "aac", "-b:a", "192k",
            "-t", f"{video_duration(video):.3f}", "-movflags", "+faststart", str(output),
        ], check=True)

    return {"output": str(output.resolve()), "duration": video_duration(video), **stats}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    parser.add_argument("--video", type=Path)
    parser.add_argument("--srt", type=Path)
    parser.add_argument("--audio-dir", type=Path)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--no-speed-adjust", action="store_true")
    parser.add_argument("--max-speed", type=float, default=1.25)
    parser.add_argument("--missing", choices=("error", "silence"), default="error")
    args = parser.parse_args()
    try:
        require_dependencies()
        if args.check:
            print(json.dumps({"ok": True}))
            return 0
        for name in ("video", "srt", "audio_dir", "output"):
            if getattr(args, name) is None:
                parser.error(f"--{name.replace('_', '-')} is required")
        for path in (args.video, args.srt, args.audio_dir):
            if not path.exists():
                raise FileNotFoundError(path)
        if args.max_speed < 1.0 or args.max_speed > 2.0:
            parser.error("--max-speed must be between 1.0 and 2.0")
        result = synchronize(args.video, args.srt, args.audio_dir, args.output, not args.no_speed_adjust, args.missing, args.max_speed)
        print(json.dumps(result, ensure_ascii=False))
        return 0
    except Exception as exc:
        print(json.dumps({"ok": False, "error": str(exc)}, ensure_ascii=False), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
