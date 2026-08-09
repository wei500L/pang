"""Render a PDF and OCR every page into one reviewable UTF-8 draft.

The source PDF is read only.  Each page is tagged so a later editorial pass can
remove page furniture and merge an article across a page boundary.
"""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path


def main() -> int:
    if len(sys.argv) not in (3, 5):
        print("Usage: ocr_pdf_to_text.py INPUT_PDF OUTPUT_TEXT [START_PAGE END_PAGE]", file=sys.stderr)
        return 2
    source = Path(sys.argv[1])
    output = Path(sys.argv[2])
    start = int(sys.argv[3]) if len(sys.argv) == 5 else 1
    end = int(sys.argv[4]) if len(sys.argv) == 5 else None
    images = output.parent / f"{output.stem}-pages"
    images.mkdir(parents=True, exist_ok=True)
    pages = sorted(images.glob("page-*.png"))
    if not pages:
        command = ["pdftoppm", "-r", "150", "-png"]
        if start > 1:
            command.extend(["-f", str(start)])
        if end is not None:
            command.extend(["-l", str(end)])
        command.extend(["--", str(source), str(images / "page")])
        subprocess.run(
            command,
            check=True,
        )
        pages = sorted(images.glob("page-*.png"))
    if not pages:
        raise RuntimeError("No page images were rendered")
    if end is None:
        end = start + len(pages) - 1
    if start < 1 or end < start or len(pages) != end - start + 1:
        raise ValueError(f"Requested pages {start}-{end}, rendered {len(pages)} pages")
    result: list[str] = []
    for number, image in enumerate(pages, start=start):
        completed = subprocess.run(
            [r"C:\Program Files\Tesseract-OCR\tesseract.exe", str(image), "stdout", "-l", "chi_sim", "--psm", "6"],
            check=True,
            capture_output=True,
            text=True,
            encoding="utf-8",
        )
        result.extend((f"\n\n===== OCR PAGE {number} =====\n", completed.stdout.strip()))
        output.write_text("\n".join(result).strip() + "\n", encoding="utf-8")
        print(f"OCR {number}/{len(pages)}", flush=True)
    output.write_text("\n".join(result).strip() + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
