from io import BytesIO

from pypdf import PdfReader


def extract_pages(pdf_file: BytesIO) -> list[dict]:
    """Extract text from each page of a PDF.

    Returns a list of dicts with 'page_number' (1-indexed) and 'text' keys.
    Raises ValueError if the file is empty or not a valid PDF.
    """
    try:
        content = pdf_file.read()
        if not content:
            raise ValueError("empty or invalid PDF")
        pdf_file.seek(0)
        reader = PdfReader(pdf_file)
    except Exception as e:
        if "empty or invalid" in str(e):
            raise
        raise ValueError(f"empty or invalid PDF: {e}")

    pages = []
    for i, page in enumerate(reader.pages):
        text = page.extract_text() or ""
        pages.append({"page_number": i + 1, "text": text})

    return pages
