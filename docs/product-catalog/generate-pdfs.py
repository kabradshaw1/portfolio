#!/usr/bin/env python3
"""Generate PDFs from Markdown source files in docs/product-catalog/.

Usage:
    pip install fpdf2
    python docs/product-catalog/generate-pdfs.py
"""

import re
from pathlib import Path

from fpdf import FPDF


_UNICODE_REPLACEMENTS = {
    "\u2014": "--",   # em dash
    "\u2013": "-",    # en dash
    "\u2018": "'",    # left single quote
    "\u2019": "'",    # right single quote
    "\u201c": '"',    # left double quote
    "\u201d": '"',    # right double quote
    "\u2026": "...",  # ellipsis
    "\u00b0": " deg", # degree sign
    "\u00d7": "x",    # multiplication sign
    "\u00ae": "(R)",  # registered trademark
    "\u2122": "(TM)", # trademark
    "\u00a9": "(C)",  # copyright
    "\u00b1": "+/-",  # plus-minus
    "\u00bc": "1/4",  # fraction 1/4
    "\u00bd": "1/2",  # fraction 1/2
    "\u00be": "3/4",  # fraction 3/4
}


def _sanitize(text: str) -> str:
    """Replace non-latin-1 characters with ASCII equivalents."""
    for char, replacement in _UNICODE_REPLACEMENTS.items():
        text = text.replace(char, replacement)
    # Drop any remaining non-latin-1 characters
    return text.encode("latin-1", errors="replace").decode("latin-1")


def md_to_pdf(md_path: Path, pdf_path: Path) -> None:
    """Convert a Markdown file to a simple PDF."""
    pdf = FPDF()
    pdf.set_auto_page_break(auto=True, margin=20)
    pdf.add_page()

    text = md_path.read_text(encoding="utf-8")

    for line in text.split("\n"):
        stripped = line.strip()

        if stripped.startswith("# "):
            pdf.set_font("Helvetica", "B", 18)
            pdf.ln(2)
            pdf.set_x(pdf.l_margin)
            pdf.multi_cell(0, 12, _sanitize(stripped[2:]))
            pdf.ln(4)
        elif stripped.startswith("## "):
            pdf.set_font("Helvetica", "B", 14)
            pdf.ln(4)
            pdf.set_x(pdf.l_margin)
            pdf.multi_cell(0, 10, _sanitize(stripped[3:]))
            pdf.ln(2)
        elif stripped.startswith("### "):
            pdf.set_font("Helvetica", "B", 12)
            pdf.ln(2)
            pdf.set_x(pdf.l_margin)
            pdf.multi_cell(0, 8, _sanitize(stripped[4:]))
            pdf.ln(1)
        elif stripped.startswith("- ") or stripped.startswith("* "):
            pdf.set_font("Helvetica", "", 10)
            content = _sanitize(re.sub(r"\*\*(.+?)\*\*", r"\1", stripped[2:]))
            pdf.set_x(pdf.l_margin + 6)
            pdf.multi_cell(pdf.w - pdf.l_margin - pdf.r_margin - 6, 6, f"* {content}")
        elif re.match(r"^\d+\.", stripped):
            pdf.set_font("Helvetica", "", 10)
            content = _sanitize(re.sub(r"\*\*(.+?)\*\*", r"\1", stripped))
            pdf.set_x(pdf.l_margin + 6)
            pdf.multi_cell(pdf.w - pdf.l_margin - pdf.r_margin - 6, 6, content)
        elif stripped == "":
            pdf.ln(3)
        else:
            pdf.set_font("Helvetica", "", 10)
            content = _sanitize(re.sub(r"\*\*(.+?)\*\*", r"\1", stripped))
            pdf.set_x(pdf.l_margin)
            pdf.multi_cell(0, 6, content)

    pdf.output(str(pdf_path))
    print(f"  {pdf_path.name} ({pdf.pages_count} pages)")


def main() -> None:
    catalog_dir = Path(__file__).parent
    md_files = sorted(catalog_dir.glob("*.md"))

    if not md_files:
        print("No Markdown files found in", catalog_dir)
        return

    print(f"Generating {len(md_files)} PDFs...")
    for md_file in md_files:
        pdf_file = md_file.with_suffix(".pdf")
        md_to_pdf(md_file, pdf_file)

    print("Done.")


if __name__ == "__main__":
    main()
