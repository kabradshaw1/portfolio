import io

import pytest
from app.pdf_parser import extract_pages


def test_extract_pages_returns_list_of_dicts(sample_pdf_bytes):
    pages = extract_pages(io.BytesIO(sample_pdf_bytes))
    assert isinstance(pages, list)
    assert len(pages) == 2
    for page in pages:
        assert "page_number" in page
        assert "text" in page


def test_extract_pages_page_numbers_are_1_indexed(sample_pdf_bytes):
    pages = extract_pages(io.BytesIO(sample_pdf_bytes))
    assert pages[0]["page_number"] == 1
    assert pages[1]["page_number"] == 2


def test_extract_pages_empty_bytes_raises():
    with pytest.raises(ValueError, match="empty or invalid"):
        extract_pages(io.BytesIO(b""))


def test_extract_pages_invalid_pdf_raises():
    with pytest.raises(ValueError, match="empty or invalid"):
        extract_pages(io.BytesIO(b"not a pdf"))
