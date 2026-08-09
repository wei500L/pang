"""Build a page-traceable, visually reviewed reading draft for one scanned book.

The PDF text layer supplies page-local body text; the rules below remove scan
furniture and repair recurring OCR substitutions that were confirmed against
the rendered pages.  It deliberately keeps an HTML page anchor for every PDF
page so future corrections can always be checked against the source image.
"""

from __future__ import annotations

import re
import sys
from dataclasses import dataclass
from pathlib import Path

import pymupdf


@dataclass
class Line:
    text: str
    y0: float
    y1: float


BOOK_TITLE = "胖东来，你要怎么学？"

CHAPTER_OPENERS = {
    36: ("## 第一章 许昌出了个胖东来", {"第一章", "许昌出了个胖东来"}),
    74: ("## 第二章 灯一样的企业", {"第二章", "灯一样的企业"}),
    114: ("## 第三章 真正的人性化管理", {"第三章", "真正的人性化管理"}),
    162: ("## 第四章 让顾客满意", set()),
    210: ("## 第五章 文化立企文化强企", set()),
}

SECTION_OPENERS = {
    10: ("## 爱的传道者", {"爱的传道者"}),
    14: ("## 寻找和发现中国的企业思想家", {"寻找和发现中国的企业思想家"}),
    22: ("## 写在前面", {"写在前面"}),
    164: ("### 回归商业本质", {"回归商业本质"}),
    212: ("### “东来特色”的企业文化", {"“东来特色”的企业文化"}),
    253: ("## 后记", {"后记"}),
}

REPEATED_FURNITURE = {
    "第一章", "许昌出了个胖东来", "第二章", "灯一样的企业", "灯—样的企业",
    "第三章", "真正的人性化管理", "第四章", "让顾客满意", "第五章",
    "文化立企文化强企", "文化立企，文化强企", "DL 胖东来，", "你要怎么学？",
    "胖东来，", "爱的传道者", "寻找和发现中国的企业思想家", "写在前面", "后记",
    "J", "I", "_",
}

CORRECTIONS = (
    ("商度敬业", "高度敬业"),
    ("入生", "人生"), ("做入", "做人"), ("他入", "他人"),
    ("入民", "人民"), ("入际", "人际"), ("入本", "人本"),
    ("入力", "人力"), ("个入", "个人"), ("全入", "全人"),
    ("由千", "由于"), ("至千", "至于"), ("关千", "关于"),
    ("基千", "基于"), ("千东来", "于东来"), ("千先生", "于先生"),
    ("千201", "于201"), ("千20", "于20"), ("千19", "于19"),
    ("坎坎坰坰", "坎坎坷坷"), ("坎坷坷", "坎坎坷坷"), ("坎坰", "坎坷"),
    ("明世理", "明事理"), ("激清", "激情"), ("清况", "情况"),
    ("自已", "自己"), ("巳", "已"), ("支待", "支持"),
    ("颜千", "源于"), ("浩渤", "浩瀚"), ("腊玛占猿", "腊玛古猿"),
    ("劝沔虽", "500强"), ("企业萝想家", "企业思想家"),
    ("清操", "情操"), ("福扯", "福祉"), ("拫上一口", "喝上一口"),
    ("劣根性", "劣根性"),
    ("普通入", "普通人"), ("下的入", "下的人"), ("入渺小", "人渺小"),
    ("前入猿", "前人猿"), ("现代入类", "现代人类"), ("入类", "人类"),
    ("和入性", "和人性"), ("中国入", "中国人"), ("多的入", "多的人"),
    ("这群入", "这群人"), ("令入", "令人"), ("把入类", "把人类"),
    ("领头入", "领头人"), ("万入去", "万人去"), ("的入生", "的人生"),
    ("创始入", "创始人"), ("第一入", "第一人"), ("众入", "众人"),
    ("千东来", "于东来"),
    ("远多千", "远多于"), ("专注千", "专注于"), ("对千", "对于"),
    ("属千", "属于"), ("源千", "源于"), ("限千", "限于"),
    ("出千", "出于"), ("异千", "异于"), ("终千", "终于"),
    ("出生千", "出生于"), ("敢千", "敢于"), ("基千", "基于"),
    ("应用千", "应用于"), ("取决千", "取决于"), ("来自千", "来自于"),
    ("忙千", "忙于"), ("还在千", "还在于"), ("特属千", "特属于"),
    ("等同千", "等同于"), ("止千", "止于"), ("归属千", "归属于"),
    ("臻千", "臻于"), ("不等千", "不等于"), ("起千", "起于"),
    ("胜于千里", "胜于千里"), ("于公里", "于公里"),
    ("WorldHappinessReport", "World Happiness Report"), ("驱动力30", "驱动力3.0"),
    ("则是不懈追求", "则是我不懈追求"),
    ("经区域", "经营区域"),
    ("—个", "一个"), ("—些", "一些"), ("—定", "一定"),
    ("—样", "一样"), ("—种", "一种"), ("—次", "一次"),
    ("—直", "一直"), ("—生", "一生"), ("—路", "一路"),
    ("—方面", "一方面"), ("—体", "一体"), ("—半", "一半"),
    ("—切", "一切"), ("—度", "一度"),
    # PaddleOCR occasionally emits ordinary print as inline LaTex.  These
    # replacements were checked on the corresponding source pages.
    ("D $ \\underline{L} $ 胖东来，你要怎么学？", ""),
    ("探 $ \\underset{\\cdot}{讨} $", "探讨"),
    ("IGA $ ^{①} $中国", "IGA①中国"),
    ("中国实友会 $ ^{①} $", "中国实友会①"),
    ("与 $ 68^{\\circ} $C的许昌邂逅", "与 68°C 的许昌邂逅"),
    ("$ 68^{\\circ} $C", "68°C"),
    ("$ 65^{\\circ} $C~68 $ ^{\\circ} $C", "65°C～68°C"),
    ("・・・・・・", "……"),
)


def correct_ocr(text: str) -> str:
    for old, new in CORRECTIONS:
        text = text.replace(old, new)
    return text


def clean(text: str) -> str:
    return correct_ocr(re.sub(r"[\t \u3000]+", "", text).strip())


def lines_for_page(page: pymupdf.Page) -> list[Line]:
    lines: list[Line] = []
    for block in page.get_text("dict", sort=True)["blocks"]:
        if block["type"] != 0:
            continue
        for raw_line in block["lines"]:
            text = clean("".join(span["text"] for span in raw_line["spans"]))
            if text:
                bbox = raw_line["bbox"]
                lines.append(Line(text, bbox[1], bbox[3]))
    return lines


def is_noise(text: str, page_number: int) -> bool:
    if text in REPEATED_FURNITURE and page_number not in CHAPTER_OPENERS:
        return True
    if text in {"DL胖东来，", "DL胖东来，你要怎么学？"}:
        return True
    if re.fullmatch(r"[-—–·•\s]*\d{1,4}[-—–·•\s]*", text):
        return True
    if re.fullmatch(r"[·.\-—–]*\d?[ivxlcdmIVXLCDM]{1,8}[·.\-—–]*", text):
        return True
    if re.fullmatch(r"F-\d+(?:\.\d+)?", text):
        return True
    # Stray glyphs from photo and diagram OCR, never body copy.
    if len(text) == 1 and text in "JIL_@~，,。·•":
        return True
    if re.fullmatch(r"[lI1J;:,. ]{0,4}L?胖东来，?", text):
        return True
    return False


def page_to_paragraphs(lines: list[Line], page_number: int, skip: set[str]) -> list[str]:
    result: list[str] = []
    current: list[str] = []
    previous: Line | None = None

    def flush() -> None:
        if current:
            # Some OCR substitutions span two visual lines, so normalize again
            # after the line fragments have been joined.
            paragraph = clean("".join(current)).strip()
            if paragraph:
                result.append(paragraph)
            current.clear()

    paragraph_gap = 25 if page_number in {163, 211} else 13
    for line in lines:
        if line.text in skip or is_noise(line.text, page_number):
            continue
        # Running heads and folios are placed in the outer margins.  All true
        # chapter/section opening titles are emitted above from their confirmed
        # page map, so these margins can be safely discarded here.
        if line.y0 < 115 or line.y1 > 750:
            continue
        # Vertical-chart labels and isolated OCR artefacts are not readable prose.
        if len(line.text) == 1 and "\u4e00" <= line.text <= "\u9fff":
            continue
        if previous is not None and line.y0 - previous.y1 > paragraph_gap:
            flush()
        current.append(line.text)
        previous = line
    flush()
    return result


def early_pages(page_number: int) -> list[str] | None:
    if page_number == 1:
        return [
            "# 《胖东来，你要怎么学？》",
            "",
            "> 原文件：胖东来，你要怎么学？.pdf  ",
            "> PDF 总页数：256",
            "",
            "## 书目信息",
            "",
            "- 作者：王慧中",
            "- 审订作序：于东来",
            "- 出版社：龙门书局",
            "- 出版时间：2014 年 8 月（首版印刷：2014 年 9 月）",
            "- ISBN：978-7-5088-4343-8",
            "",
            "## 封面文案",
            "",
            "> 把员工当成完整意义上的人，让创造财富的人分享财富，就培养出了能干会玩、高度敬业的员工。",
            ">\n> 用爱诠释工匠精神，把零售做成一门艺术，就拥有了最忠诚的顾客，就重塑了一个地区的商业生态。",
        ]
    captions = {
        4: "图注：1995 年创业至今，胖东来完成了数次蜕变，却始终在用爱经营企业。",
        6: "图注：从超市百货到自营餐饮，胖东来都在以温馨的环境赢得顾客的满意。",
        7: "图注：胖东来三星级员工——① 消防部董小海；② 食品部刘改香；③ 电器部孙莉；④ 维修部郑江龙；⑤ 医药部陈丽；⑥ 保洁部张瑞英。",
        8: "图注：许昌胖东来时代广场员工活动中心的休闲设施一应俱全。",
        9: "图注：和员工在一起的“东来哥”。",
    }
    if page_number in captions:
        return [captions[page_number]]
    if 2 <= page_number <= 3 or page_number == 5:
        return []
    if page_number == 13:
        return [
            "这本《胖东来，你要怎么学？》能给你带来一些触动，并对今后的工作有所启发，我相信，不仅你自己会更加自信、幸福，也必将惠及你身边的千千万万个家庭。若如此，我一定要衷心感谢你帮助我向着“善良、勤奋、自由和博爱”人生信仰更近了一步！",
            "",
            "于东来",
            "胖东来商贸集团董事长",
            "2014 年 7 月",
        ]
    return None


def paddle_paragraphs(ocr_root: Path, page_number: int, skip: set[str]) -> list[str]:
    """Read the page-level PaddleOCR-VL result, omitting image-only HTML."""
    matches = list(ocr_root.rglob(f"page-{page_number:04d}.md"))
    if len(matches) != 1:
        raise FileNotFoundError(
            f"Expected exactly one PaddleOCR page {page_number}, found {len(matches)}"
        )
    raw = matches[0].read_text(encoding="utf-8")
    raw = re.sub(r"<div[^>]*>\s*<img[^>]*>\s*</div>", "", raw, flags=re.S)
    result: list[str] = []
    for line in raw.splitlines():
        text = correct_ocr(line.strip())
        if not text or "<img " in text or text.startswith("<div "):
            continue
        plain = re.sub(r"^#{1,6}\s*", "", text).strip()
        if plain in skip:
            continue
        # The book title and chapter running heads are already represented by
        # the trusted page map, not by per-page OCR headings.
        if plain in REPEATED_FURNITURE:
            continue
        result.append(text)
    return result


def build(source: Path, destination: Path, ocr_root: Path | None = None) -> None:
    document = pymupdf.open(source)
    if document.page_count != 256:
        raise ValueError(f"Expected 256 pages, got {document.page_count}")

    output: list[str] = []
    for page_number, page in enumerate(document, start=1):
        output.extend([f"<!-- PDF 页：{page_number} -->", ""])
        special = early_pages(page_number)
        if special is not None:
            output.extend(special)
            output.append("")
            continue

        # The original contents spans four designed pages. Markdown headings are
        # a better navigation layer, so keep only their source anchors.
        if 18 <= page_number <= 21:
            continue

        skip: set[str] = set()
        if page_number in CHAPTER_OPENERS:
            heading, skip = CHAPTER_OPENERS[page_number]
            output.extend([heading, ""])
            if page_number == 162:
                output.extend(["> 丰富的商品、合理的价格、温馨的环境、完善的服务，其中每一个方面，都有着极其丰富的内涵。", ""])
                continue
            if page_number == 210:
                output.extend(["> 于东来希望，胖东来的企业文化不仅能指导员工如何工作，还要能指导员工如何生活。", ""])
                continue
        elif page_number in SECTION_OPENERS:
            heading, skip = SECTION_OPENERS[page_number]
            output.extend([heading, ""])

        paragraphs = (
            paddle_paragraphs(ocr_root, page_number, skip)
            if ocr_root is not None
            else page_to_paragraphs(lines_for_page(page), page_number, skip)
        )
        for paragraph in paragraphs:
            output.extend([paragraph, ""])

    # Keep each source-page anchor on its own line.  Inserting an HTML comment
    # in the middle of a Chinese word makes literal retrieval needlessly fail.
    result = "\n".join(output).rstrip() + "\n"
    destination.parent.mkdir(parents=True, exist_ok=True)
    temporary = destination.with_suffix(destination.suffix + ".tmp")
    temporary.write_text(result, encoding="utf-8")
    temporary.replace(destination)


if __name__ == "__main__":
    if len(sys.argv) not in {3, 4}:
        raise SystemExit(
            "Usage: repair_how_to_learn.py SOURCE_PDF OUTPUT_MARKDOWN [PADDLE_OCR_ROOT]"
        )
    build(
        Path(sys.argv[1]),
        Path(sys.argv[2]),
        Path(sys.argv[3]) if len(sys.argv) == 4 else None,
    )
