"""Upload PDF page ranges to PaddleOCR-VL in files below the API size limit.

The source PDF is read-only.  Requested page ranges are materialized into
temporary PDF parts, then uploaded serially with multipart/form-data.  Each
completed part is cached with its JSONL result and a Markdown draft per source
PDF page, making interrupted long-running book conversion resumable.
"""

from __future__ import annotations

import argparse
import io
import json
import os
import sys
import time
import traceback
from pathlib import Path

import requests
from pypdf import PdfReader, PdfWriter


JOB_URL = "https://paddleocr.aistudio-app.com/api/v2/ocr/jobs"
TOKEN = os.environ["PADDLEOCR_TOKEN"]
MODEL = "PaddleOCR-VL-1.6"
OPTIONAL_PAYLOAD = {
    "useDocOrientationClassify": False,
    "useDocUnwarping": False,
    "useChartRecognition": False,
}
DEFAULT_MAX_BYTES = 48 * 1024 * 1024


def request_with_retry(method: str, url: str, *, attempts: int = 6, **kwargs):
    last_error: Exception | None = None
    for attempt in range(1, attempts + 1):
        try:
            response = requests.request(method, url, timeout=(30, 300), **kwargs)
            if response.status_code == 200:
                return response
            last_error = RuntimeError(
                f"HTTP {response.status_code}: {response.text[:1000]}"
            )
        except requests.RequestException as exc:
            last_error = exc
        if attempt < attempts:
            time.sleep(min(10 * attempt, 60))
    raise RuntimeError(f"Request failed after {attempts} attempts: {last_error}")


def write_pdf_part(reader: PdfReader, pages: list[int], destination: Path) -> int:
    writer = PdfWriter()
    for page_number in pages:
        writer.add_page(reader.pages[page_number - 1])
    destination.parent.mkdir(parents=True, exist_ok=True)
    with destination.open("wb") as stream:
        writer.write(stream)
    return destination.stat().st_size


def serialized_page_size(reader: PdfReader, page_number: int) -> int:
    writer = PdfWriter()
    writer.add_page(reader.pages[page_number - 1])
    buffer = io.BytesIO()
    writer.write(buffer)
    return buffer.tell()


def split_page_range(
    source: Path,
    parts_dir: Path,
    start_page: int,
    end_page: int,
    maximum_bytes: int,
) -> list[dict[str, object]]:
    reader = PdfReader(str(source))
    if not 1 <= start_page <= end_page <= len(reader.pages):
        raise ValueError(
            f"Invalid range {start_page}-{end_page}; PDF has {len(reader.pages)} pages"
        )

    manifest_path = parts_dir / "manifest.json"
    if manifest_path.is_file():
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        if (
            manifest.get("source") == str(source)
            and manifest.get("start_page") == start_page
            and manifest.get("end_page") == end_page
            and manifest.get("maximum_bytes") == maximum_bytes
            and all((parts_dir / part["filename"]).is_file() for part in manifest["parts"])
        ):
            return manifest["parts"]

    # Keep a conservative margin for PDF object overhead and multipart upload.
    page_sizes = {
        page_number: serialized_page_size(reader, page_number)
        for page_number in range(start_page, end_page + 1)
    }
    budget = int(maximum_bytes * 0.88)
    groups: list[list[int]] = []
    current: list[int] = []
    current_size = 0
    for page_number in range(start_page, end_page + 1):
        page_size = page_sizes[page_number]
        if page_size > maximum_bytes:
            raise RuntimeError(
                f"Single source page {page_number} exceeds upload limit: {page_size} bytes"
            )
        if current and current_size + page_size > budget:
            groups.append(current)
            current = []
            current_size = 0
        current.append(page_number)
        current_size += page_size
    if current:
        groups.append(current)

    parts: list[dict[str, object]] = []
    for index, pages in enumerate(groups, start=1):
        pending = [pages]
        while pending:
            candidate = pending.pop(0)
            filename = (
                f"part-{index:03d}-p{candidate[0]:04d}-p{candidate[-1]:04d}.pdf"
            )
            destination = parts_dir / filename
            size = write_pdf_part(reader, candidate, destination)
            if size > maximum_bytes and len(candidate) > 1:
                destination.unlink(missing_ok=True)
                midpoint = len(candidate) // 2
                pending.insert(0, candidate[midpoint:])
                pending.insert(0, candidate[:midpoint])
                continue
            if size > maximum_bytes:
                raise RuntimeError(
                    f"Generated part exceeds upload limit: {destination} ({size} bytes)"
                )
            parts.append(
                {
                    "filename": filename,
                    "first_page": candidate[0],
                    "last_page": candidate[-1],
                    "page_count": len(candidate),
                    "bytes": size,
                }
            )
            index += 1

    manifest_path.write_text(
        json.dumps(
            {
                "source": str(source),
                "start_page": start_page,
                "end_page": end_page,
                "maximum_bytes": maximum_bytes,
                "parts": parts,
            },
            ensure_ascii=False,
            indent=2,
        )
        + "\n",
        encoding="utf-8",
    )
    return parts


def submit_part(file_path: Path) -> str:
    headers = {"Authorization": f"bearer {TOKEN}"}
    data = {"model": MODEL, "optionalPayload": json.dumps(OPTIONAL_PAYLOAD)}
    with file_path.open("rb") as stream:
        response = request_with_retry(
            "POST",
            JOB_URL,
            headers=headers,
            data=data,
            files={"file": (file_path.name, stream, "application/pdf")},
        )
    return response.json()["data"]["jobId"]


def wait_for_result(job_id: str) -> str:
    headers = {"Authorization": f"bearer {TOKEN}"}
    started_at = time.monotonic()
    while True:
        response = request_with_retry("GET", f"{JOB_URL}/{job_id}", headers=headers)
        data = response.json()["data"]
        state = data["state"]
        if state == "done":
            progress = data.get("extractProgress", {})
            print(
                f"{job_id}: done, pages={progress.get('extractedPages', '?')}",
                flush=True,
            )
            return data["resultUrl"]["jsonUrl"]
        if state == "failed":
            raise RuntimeError(data.get("errorMsg", "OCR job failed"))
        if time.monotonic() - started_at > 7200:
            raise TimeoutError(f"OCR job did not finish within two hours: {job_id}")
        time.sleep(10)


def save_result(
    jsonl_url: str,
    output_dir: Path,
    first_page: int,
    expected_pages: int,
) -> None:
    output_dir.mkdir(parents=True, exist_ok=True)
    response = request_with_retry("GET", jsonl_url)
    (output_dir / "result.jsonl").write_bytes(response.content)
    all_markdown: list[str] = []
    parsed_pages = 0
    for line in response.text.splitlines():
        if not line.strip():
            continue
        payload = json.loads(line)
        for parsed_result in payload["result"]["layoutParsingResults"]:
            page_number = first_page + parsed_pages
            markdown = parsed_result["markdown"]["text"].strip()
            (output_dir / f"page-{page_number:04d}.md").write_text(
                markdown + "\n", encoding="utf-8"
            )
            all_markdown.append(f"<!-- PDF page: {page_number} -->\n\n{markdown}")
            parsed_pages += 1
    if parsed_pages != expected_pages:
        raise RuntimeError(
            f"Expected {expected_pages} OCR pages, got {parsed_pages} in {output_dir}"
        )
    (output_dir / "raw.md").write_text(
        "\n\n".join(all_markdown).strip() + "\n", encoding="utf-8"
    )


def has_complete_result(output_dir: Path, expected_pages: int) -> bool:
    result_path = output_dir / "result.jsonl"
    raw_path = output_dir / "raw.md"
    if not result_path.is_file() or not raw_path.is_file():
        return False
    return len(list(output_dir.glob("page-*.md"))) == expected_pages


def process_part(parts_dir: Path, ocr_root: Path, part: dict[str, object]) -> None:
    filename = str(part["filename"])
    first_page = int(part["first_page"])
    last_page = int(part["last_page"])
    expected_pages = int(part["page_count"])
    file_path = parts_dir / filename
    output_dir = ocr_root / file_path.stem
    if has_complete_result(output_dir, expected_pages):
        # A terminal console write can fail after results have been persisted.
        # Treat the complete cache as authoritative on resume and remove its
        # stale diagnostic so it is not reported as a failed upload.
        (output_dir / "error.txt").unlink(missing_ok=True)
        (output_dir / "job_id.txt").unlink(missing_ok=True)
        print(f"Skipping completed part: {file_path.name}", flush=True)
        return
    output_dir.mkdir(parents=True, exist_ok=True)
    job_id_path = output_dir / "job_id.txt"
    try:
        if job_id_path.is_file():
            job_id = job_id_path.read_text(encoding="utf-8").strip()
            print(f"Resuming {file_path.name}: {job_id}", flush=True)
        else:
            job_id = submit_part(file_path)
            job_id_path.write_text(f"{job_id}\n", encoding="utf-8")
            print(
                f"Submitted {file_path.name} (pages {first_page}-{last_page}): {job_id}",
                flush=True,
            )
        save_result(wait_for_result(job_id), output_dir, first_page, expected_pages)
        job_id_path.unlink(missing_ok=True)
        (output_dir / "error.txt").unlink(missing_ok=True)
        print(f"Saved OCR draft: {output_dir}", flush=True)
    except Exception:
        (output_dir / "error.txt").write_text(traceback.format_exc(), encoding="utf-8")
        raise


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("source_pdf", type=Path)
    parser.add_argument("temporary_root", type=Path)
    parser.add_argument("--start-page", type=int, default=1)
    parser.add_argument("--end-page", type=int)
    parser.add_argument("--max-bytes", type=int, default=DEFAULT_MAX_BYTES)
    args = parser.parse_args()
    source = args.source_pdf.resolve()
    if not source.is_file():
        raise FileNotFoundError(source)
    reader = PdfReader(str(source))
    end_page = args.end_page or len(reader.pages)
    parts_dir = args.temporary_root / "parts"
    ocr_root = args.temporary_root / "ocr"
    parts = split_page_range(
        source,
        parts_dir,
        args.start_page,
        end_page,
        args.max_bytes,
    )
    for part in parts:
        process_part(parts_dir, ocr_root, part)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
