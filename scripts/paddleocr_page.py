import json
import os
import sys
import time
import traceback
from pathlib import Path

import requests


JOB_URL = "https://paddleocr.aistudio-app.com/api/v2/ocr/jobs"
TOKEN = os.environ["PADDLEOCR_TOKEN"]
MODEL = "PaddleOCR-VL-1.6"

OPTIONAL_PAYLOAD = {
    "useDocOrientationClassify": False,
    "useDocUnwarping": False,
    "useChartRecognition": False,
}


def request_with_retry(method, url, *, attempts=4, **kwargs):
    last_error = None
    for attempt in range(1, attempts + 1):
        try:
            response = requests.request(method, url, timeout=(30, 300), **kwargs)
            if response.status_code == 200:
                return response
            last_error = RuntimeError(f"HTTP {response.status_code}: {response.text[:1000]}")
        except requests.RequestException as exc:
            last_error = exc
        if attempt < attempts:
            time.sleep(min(5 * attempt, 20))
    raise RuntimeError(f"Request failed after {attempts} attempts: {last_error}")


def submit_page(file_path: Path) -> str:
    headers = {"Authorization": f"bearer {TOKEN}"}
    data = {"model": MODEL, "optionalPayload": json.dumps(OPTIONAL_PAYLOAD)}
    with file_path.open("rb") as stream:
        files = {"file": (file_path.name, stream, "application/pdf")}
        response = request_with_retry("POST", JOB_URL, headers=headers, data=data, files=files)
    return response.json()["data"]["jobId"]


def wait_for_result(job_id: str) -> str:
    headers = {"Authorization": f"bearer {TOKEN}"}
    started_at = time.monotonic()
    maximum_wait_seconds = 1800
    while True:
        response = request_with_retry("GET", f"{JOB_URL}/{job_id}", headers=headers)
        data = response.json()["data"]
        state = data["state"]
        if state == "pending":
            print(f"{job_id}: pending", flush=True)
        elif state == "running":
            progress = data.get("extractProgress", {})
            print(f"{job_id}: running {progress.get('extractedPages', 0)}/{progress.get('totalPages', 1)}", flush=True)
        elif state == "done":
            progress = data.get("extractProgress", {})
            print(f"{job_id}: done, pages={progress.get('extractedPages', 1)}", flush=True)
            return data["resultUrl"]["jsonUrl"]
        elif state == "failed":
            raise RuntimeError(f"OCR job failed: {data.get('errorMsg', 'unknown error')}")
        else:
            raise RuntimeError(f"Unexpected OCR job state: {state}")
        if time.monotonic() - started_at > maximum_wait_seconds:
            raise TimeoutError(f"OCR job did not finish within {maximum_wait_seconds} seconds")
        time.sleep(5)


def save_result(jsonl_url: str, output_dir: Path) -> None:
    output_dir.mkdir(parents=True, exist_ok=True)
    response = request_with_retry("GET", jsonl_url)
    (output_dir / "result.jsonl").write_bytes(response.content)
    markdown_parts = []
    parsed_pages = 0
    for line_number, line in enumerate(response.text.splitlines(), start=1):
        line = line.strip()
        if not line:
            continue
        payload = json.loads(line)
        result = payload["result"]
        for parsed_result in result["layoutParsingResults"]:
            markdown = parsed_result["markdown"]
            markdown_parts.append(markdown["text"].strip())
            for image_path, image_url in markdown.get("images", {}).items():
                destination = output_dir / image_path
                destination.parent.mkdir(parents=True, exist_ok=True)
                destination.write_bytes(request_with_retry("GET", image_url).content)
            for image_name, image_url in parsed_result.get("outputImages", {}).items():
                destination = output_dir / f"{image_name}_{parsed_pages:04d}.jpg"
                destination.write_bytes(request_with_retry("GET", image_url).content)
            parsed_pages += 1
    if parsed_pages != 1:
        raise RuntimeError(f"Expected one OCR page, got {parsed_pages}")
    (output_dir / "raw.md").write_text("\n\n".join(markdown_parts).strip() + "\n", encoding="utf-8")


def has_complete_cached_result(output_dir: Path) -> bool:
    """Return true only for a non-empty, single-page persisted OCR result."""
    raw_path = output_dir / "raw.md"
    jsonl_path = output_dir / "result.jsonl"
    if not raw_path.is_file() or not jsonl_path.is_file():
        return False
    parsed_pages = 0
    try:
        for line in jsonl_path.read_text(encoding="utf-8").splitlines():
            if not line.strip():
                continue
            result = json.loads(line)["result"]
            parsed_pages += len(result["layoutParsingResults"])
    except (OSError, UnicodeDecodeError, json.JSONDecodeError, KeyError, TypeError):
        return False
    return parsed_pages == 1


def main() -> int:
    if len(sys.argv) != 3:
        print("Usage: paddleocr_page.py PAGE_PDF OUTPUT_DIRECTORY", file=sys.stderr)
        return 2
    file_path = Path(sys.argv[1]).resolve()
    output_dir = Path(sys.argv[2]).resolve()
    if not file_path.is_file():
        raise FileNotFoundError(file_path)
    if has_complete_cached_result(output_dir):
        print(f"Skipping completed OCR page: {file_path.name}", flush=True)
        return 0
    print(f"Processing page: {file_path.name}", flush=True)
    try:
        output_dir.mkdir(parents=True, exist_ok=True)
        job_id_path = output_dir / "job_id.txt"
        if job_id_path.is_file():
            job_id = job_id_path.read_text(encoding="utf-8").strip()
            if not job_id:
                raise RuntimeError(f"Empty persisted job id: {job_id_path}")
            print(f"Resuming job: {job_id}", flush=True)
        else:
            job_id = submit_page(file_path)
            job_id_path.write_text(f"{job_id}\n", encoding="utf-8")
            print(f"Submitted job: {job_id}", flush=True)
        save_result(wait_for_result(job_id), output_dir)
        (output_dir / "error.txt").unlink(missing_ok=True)
        job_id_path.unlink(missing_ok=True)
        print(f"Saved OCR draft: {output_dir}", flush=True)
    except Exception as exc:
        output_dir.mkdir(parents=True, exist_ok=True)
        (output_dir / "error.txt").write_text(traceback.format_exc(), encoding="utf-8")
        raise
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
