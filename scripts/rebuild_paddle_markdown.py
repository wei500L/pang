"""Create a page-traceable Markdown reading draft from PaddleOCR-VL page files."""

from __future__ import annotations

import re
import sys
from pathlib import Path

import pymupdf


def clean_page(text: str, book_title: str) -> list[str]:
    # The original image crops are not published with this repository. Keep
    # their surrounding OCR text, but never leave a broken HTML image URL.
    text = re.sub(r"<div[^>]*>\s*<img[^>]*>\s*</div>", "", text, flags=re.S)
    output: list[str] = []
    for line in text.splitlines():
        line = line.strip()
        if not line or "<img " in line or line.startswith("<div "):
            continue
        plain = re.sub(r"^#{1,6}\s*", "", line).strip()
        if plain in {book_title, f"《{book_title}》"}:
            continue
        # One document-level H1 already exists; preserve the source hierarchy
        # while avoiding duplicate H1 headings from title pages.
        if line.startswith("# "):
            line = "## " + line[2:]
        output.append(line)
    return output


def rebuild(source_pdf: Path, ocr_root: Path, destination: Path) -> None:
    document = pymupdf.open(source_pdf)
    page_count = document.page_count
    files = {int(match.group(1)): path for path in ocr_root.rglob("page-*.md")
             if (match := re.fullmatch(r"page-(\d{4})\.md", path.name))}
    expected = set(range(1, page_count + 1))
    if set(files) != expected:
        missing = sorted(expected - set(files))
        extra = sorted(set(files) - expected)
        raise ValueError(f"OCR page mismatch; missing={missing[:8]}, extra={extra[:8]}")

    title = source_pdf.stem
    output = [f"# 《{title}》", "", f"> 原文件：{source_pdf.name}  ", f"> PDF 总页数：{page_count}", ""]
    for number in range(1, page_count + 1):
        output.extend([f"<!-- PDF 页：{number} -->", ""])
        for line in clean_page(files[number].read_text(encoding="utf-8"), title):
            output.extend([line, ""])

    destination.parent.mkdir(parents=True, exist_ok=True)
    temporary = destination.with_suffix(destination.suffix + ".tmp")
    temporary.write_text("\n".join(output).rstrip() + "\n", encoding="utf-8")
    temporary.replace(destination)


if __name__ == "__main__":
    if len(sys.argv) != 4:
        raise SystemExit("Usage: rebuild_paddle_markdown.py SOURCE_PDF OCR_ROOT OUTPUT_MARKDOWN")
    rebuild(Path(sys.argv[1]), Path(sys.argv[2]), Path(sys.argv[3]))
