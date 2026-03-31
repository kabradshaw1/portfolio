from app.prompt import build_rag_prompt


def test_build_rag_prompt_includes_context():
    chunks = [
        {"text": "The revenue was $1M.", "filename": "report.pdf", "page_number": 3},
    ]
    prompt = build_rag_prompt(question="What was the revenue?", chunks=chunks)
    assert "The revenue was $1M." in prompt
    assert "What was the revenue?" in prompt


def test_build_rag_prompt_includes_source_attribution():
    chunks = [
        {"text": "Some fact.", "filename": "doc.pdf", "page_number": 5},
    ]
    prompt = build_rag_prompt(question="Tell me a fact.", chunks=chunks)
    assert "doc.pdf" in prompt
    assert "5" in prompt


def test_build_rag_prompt_multiple_chunks():
    chunks = [
        {"text": "First chunk.", "filename": "a.pdf", "page_number": 1},
        {"text": "Second chunk.", "filename": "b.pdf", "page_number": 2},
    ]
    prompt = build_rag_prompt(question="Summarize.", chunks=chunks)
    assert "First chunk." in prompt
    assert "Second chunk." in prompt


def test_build_rag_prompt_empty_chunks():
    prompt = build_rag_prompt(question="Anything?", chunks=[])
    assert "Anything?" in prompt
    assert "no relevant context" in prompt.lower() or "don't have" in prompt.lower()
