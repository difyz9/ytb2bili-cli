#!/usr/bin/env python3
"""Convert YouTube rolling captions into non-repeating utterance subtitles."""

import argparse
from pathlib import Path

import pysrt


def clean(input_path: Path, output_path: Path, max_duration_ms: int) -> int:
    source = pysrt.open(str(input_path), encoding="utf-8")
    additions = []
    previous_lines = []
    for item in source:
        lines = [" ".join(line.split()) for line in item.text.splitlines() if line.strip()]
        new_lines = [line for line in lines if line not in previous_lines]
        if new_lines:
            additions.append((item.start.ordinal, item.end.ordinal, " ".join(new_lines)))
        previous_lines = lines

    chunks = []
    sentence_ends = (".", "!", "?", "。", "！", "？")
    for start, end, text in additions:
        should_start = (
            not chunks
            or end - chunks[-1][0] > max_duration_ms
            or (chunks[-1][2].endswith(sentence_ends) and start - chunks[-1][0] >= 1800)
        )
        if should_start:
            chunks.append([start, end, text])
        else:
            chunks[-1][1] = end
            chunks[-1][2] += " " + text

    output = pysrt.SubRipFile()
    for index, (start, end, text) in enumerate(chunks, start=1):
        output.append(
            pysrt.SubRipItem(
                index=index,
                start=pysrt.SubRipTime(milliseconds=start),
                end=pysrt.SubRipTime(milliseconds=end),
                text=text,
            )
        )
    output.save(str(output_path), encoding="utf-8")
    return len(output)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--max-duration", type=float, default=6.0)
    args = parser.parse_args()
    count = clean(args.input, args.output, int(args.max_duration * 1000))
    print(f"cleaned rolling subtitles: {count} entries -> {args.output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
