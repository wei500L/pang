"""Split a source PDF into single-page PDFs and OCR a serial page range.

Temporary pages and OCR artifacts are stored under the caller-supplied output
root. The source PDF is never modified. Each child invocation inherits
PADDLEOCR_TOKEN from this process and calls the official PaddleOCR API once.
"""

from __future__ import annotations

import argparse
import os
import subprocess
import sys
from pathlib import Path

from pypdf import PdfReader, PdfWriter


def write_single_page(source: Path, destination: Path, page_number: int) -> None:
    if destination.is_file():
        return
    reader = PdfReader(str(source))
    writer = PdfWriter()
    writer.add_page(reader.pages[page_number - 1])
    destination.parent.mkdir(parents=True, exist_ok=True)
    with destination.open("wb") as stream:
        writer.write(stream)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("source_pdf", type=Path)
    parser.add_argument("temporary_root", type=Path)
    parser.add_argument("start_page", type=int)
    parser.add_argument("end_page", type=int)
    args = parser.parse_args()

    if "PADDLEOCR_TOKEN" not in os.environ:
        raise RuntimeError("PADDLEOCR_TOKEN must be set in the current process")
    source = args.source_pdf.resolve()
    reader = PdfReader(str(source))
    total_pages = len(reader.pages)
    if not source.is_file() or not 1 <= args.start_page <= args.end_page <= total_pages:
        raise ValueError(f"Invalid range {args.start_page}-{args.end_page}; PDF has {total_pages} pages")

    script = Path(__file__).with_name("paddleocr_page.py")
    for page_number in range(args.start_page, args.end_page + 1):
        page_pdf = args.temporary_root / "pages" / f"page-{page_number:04d}.pdf"
        output_dir = args.temporary_root / "ocr" / f"page-{page_number:04d}"
        write_single_page(source, page_pdf, page_number)
        print(f"=== PAGE {page_number}/{total_pages} ===", flush=True)
        subprocess.run(
            [sys.executable, str(script), str(page_pdf), str(output_dir)],
            check=True,
            env=os.environ.copy(),
        )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
