"""Create faithful Markdown from PDFs that contain a complete native text layer.

This tool deliberately does not perform OCR.  It reads only embedded PDF text,
using typography and position to restore headings and prose paragraphs.
"""

from __future__ import annotations

import re
import sys
from collections import Counter
from dataclasses import dataclass
from pathlib import Path

import pymupdf


TERMINAL_PUNCTUATION = "。！？；：…"
PAGE_NUMBER = re.compile(r"^(?:[-—–·• ]*\d{1,4}[-—–·• ]*)$")


@dataclass
class Line:
    text: str
    x0: float
    y0: float
    y1: float
    size: float
    bold: bool
    page: int


def clean_text(text: str) -> str:
    return re.sub(r"[ \t\u3000]+", " ", text).strip()


def page_lines(page: pymupdf.Page, number: int) -> list[Line]:
    result: list[Line] = []
    for block in page.get_text("dict", sort=True)["blocks"]:
        if block["type"] != 0:
            continue
        for line in block["lines"]:
            spans = line["spans"]
            text = clean_text("".join(span["text"] for span in spans))
            if not text:
                continue
            max_span = max(spans, key=lambda span: span["size"])
            result.append(
                Line(
                    text=text,
                    x0=line["bbox"][0],
                    y0=line["bbox"][1],
                    y1=line["bbox"][3],
                    size=max_span["size"],
                    bold=any("bold" in span["font"].lower() for span in spans),
                    page=number,
                )
            )
    return result


def body_size(lines: list[Line]) -> float:
    """Find the most common text size, excluding short display text."""
    candidates = [round(line.size, 1) for line in lines if len(line.text) >= 20]
    if not candidates:
        candidates = [round(line.size, 1) for line in lines]
    return Counter(candidates).most_common(1)[0][0]


def heading_level(line: Line, normal_size: float) -> int | None:
    """Infer headings from display typography, never from guessed content."""
    text = line.text
    if PAGE_NUMBER.fullmatch(text):
        return None
    if len(text) > 60:
        return None
    # Large type is a reliable heading signal in these designed PDFs.
    if line.size >= normal_size * 1.60:
        return 2
    if line.size >= normal_size * 1.28:
        return 3
    # A short bold numbered phrase is usually a lower-level heading.
    if line.bold and len(text) <= 34 and re.match(
        r"^(?:第[一二三四五六七八九十百]+[篇部分章节]|[（(]?[一二三四五六七八九十]+[）)、.]|\d+[、.])",
        text,
    ):
        return 4
    return None


def append_paragraph(output: list[str], fragments: list[str]) -> None:
    if not fragments:
        return
    paragraph = "".join(fragments).strip()
    if paragraph:
        output.append(paragraph)
        output.append("")
    fragments.clear()


def should_continue(previous: Line, current: Line, previous_page: int) -> bool:
    if current.page != previous_page:
        return previous.text[-1:] not in TERMINAL_PUNCTUATION
    # Lines that are part of the same visual paragraph are tightly stacked.
    if current.y0 - previous.y1 <= 8.5:
        return True
    # Indented starts mark paragraphs in this material, even when the vertical gap
    # is modest. A non-terminal line before the next line remains a continuation.
    return False


def extract_book(source: Path, destination: Path) -> tuple[int, int]:
    doc = pymupdf.open(source)
    all_lines: list[Line] = []
    for page_no, page in enumerate(doc, start=1):
        all_lines.extend(page_lines(page, page_no))
    normal_size = body_size(all_lines)

    content: list[str] = [f"# 《{source.stem}》", "", f"> 原文件：{source.name}  ", f"> PDF 总页数：{doc.page_count}", ""]
    pending: list[str] = []
    previous: Line | None = None
    last_heading: tuple[int, str] | None = None

    for line in all_lines:
        level = heading_level(line, normal_size)
        if level is not None:
            append_paragraph(content, pending)
            # Consecutive large display lines form one title, e.g. “第一部分” +
            # “企业文化”. This retains book structure without page-level headings.
            if (
                last_heading is not None
                and last_heading[0] == level
                and previous
                and line.page == previous.page
                and line.y0 - previous.y1 <= 6
            ):
                content[-2] = content[-2] + " " + line.text
                last_heading = (level, content[-2])
            else:
                if line.page == 1 and line.text == source.stem:
                    previous = line
                    continue
                heading = "#" * level + " " + line.text
                content.extend([heading, ""])
                last_heading = (level, heading)
            previous = line
            continue

        last_heading = None
        if previous is not None and pending and not should_continue(previous, line, previous.page):
            append_paragraph(content, pending)
        pending.append(line.text)
        previous = line

    append_paragraph(content, pending)
    # Keep a single final newline and write atomically after all pages are read.
    destination.parent.mkdir(parents=True, exist_ok=True)
    temp = destination.with_suffix(destination.suffix + ".tmp")
    temp.write_text("\n".join(content).rstrip() + "\n", encoding="utf-8")
    temp.replace(destination)
    return doc.page_count, len(all_lines)


def main() -> int:
    if len(sys.argv) != 3:
        print("Usage: extract_native_text_layer.py SOURCE_DIRECTORY OUTPUT_DIRECTORY", file=sys.stderr)
        return 2
    source_dir = Path(sys.argv[1])
    output_dir = Path(sys.argv[2])
    processed = 0
    for source in sorted(source_dir.glob("*.pdf")):
        doc = pymupdf.open(source)
        text_chars = sum(len(page.get_text("text").strip()) for page in doc)
        # Only process books whose text layers contain actual body text.
        if text_chars < doc.page_count * 250:
            continue
        pages, lines = extract_book(source, output_dir / f"{source.stem}.md")
        print(f"DONE\t{source.name}\tpages={pages}\tlines={lines}")
        processed += 1
    print(f"Processed {processed} native-text books.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
