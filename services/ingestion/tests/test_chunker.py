from app.chunker import chunk_pages


def test_chunk_pages_returns_list_of_dicts():
    pages = [
        {"page_number": 1, "text": "Hello world. " * 100},
        {"page_number": 2, "text": "Goodbye world. " * 100},
    ]
    chunks = chunk_pages(pages, chunk_size=200, chunk_overlap=50)
    assert isinstance(chunks, list)
    assert len(chunks) > 2  # Should produce multiple chunks
    for chunk in chunks:
        assert "text" in chunk
        assert "page_number" in chunk
        assert "chunk_index" in chunk


def test_chunk_pages_preserves_page_numbers():
    pages = [
        {"page_number": 1, "text": "Short text on page one."},
        {"page_number": 3, "text": "Short text on page three."},
    ]
    chunks = chunk_pages(pages, chunk_size=1000, chunk_overlap=0)
    page_numbers = [c["page_number"] for c in chunks]
    assert 1 in page_numbers
    assert 3 in page_numbers


def test_chunk_pages_respects_chunk_size():
    pages = [{"page_number": 1, "text": "word " * 500}]
    chunks = chunk_pages(pages, chunk_size=100, chunk_overlap=20)
    for chunk in chunks:
        # Allow some overflow due to word boundaries
        assert len(chunk["text"]) <= 150


def test_chunk_pages_empty_pages_skipped():
    pages = [
        {"page_number": 1, "text": ""},
        {"page_number": 2, "text": "Has content."},
    ]
    chunks = chunk_pages(pages, chunk_size=1000, chunk_overlap=0)
    assert all(c["page_number"] == 2 for c in chunks)


def test_chunk_pages_sequential_chunk_index():
    pages = [{"page_number": 1, "text": "Hello world. " * 100}]
    chunks = chunk_pages(pages, chunk_size=100, chunk_overlap=20)
    indices = [c["chunk_index"] for c in chunks]
    assert indices == list(range(len(indices)))
