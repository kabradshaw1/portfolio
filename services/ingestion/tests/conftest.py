import io

import pytest
from PyPDF2 import PdfWriter


@pytest.fixture
def sample_pdf_bytes() -> bytes:
    """Create a simple 2-page PDF in memory."""
    writer = PdfWriter()

    # Page 1
    writer.add_blank_page(width=612, height=792)

    # Page 2
    writer.add_blank_page(width=612, height=792)

    buffer = io.BytesIO()
    writer.write(buffer)
    buffer.seek(0)
    return buffer.read()


@pytest.fixture
def empty_bytes() -> bytes:
    return b""
