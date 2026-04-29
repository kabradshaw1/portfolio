import pytest
from app.prompt import PROMPTS, build_rag_prompt


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


def test_v1_baseline_is_registered():
    assert "v1-baseline" in PROMPTS
    template = PROMPTS["v1-baseline"]
    assert "{question}" in template
    assert "{context}" in template


def test_build_rag_prompt_uses_active_version(monkeypatch):
    monkeypatch.setattr("app.config.settings.prompt_version", "v1-baseline")
    chunks = [
        {"text": "X is a thing.", "filename": "f.pdf", "page_number": 1},
    ]
    prompt = build_rag_prompt(question="What is X?", chunks=chunks)
    assert "X" in prompt
    assert "f.pdf" in prompt


def test_build_rag_prompt_raises_for_unknown_version(monkeypatch):
    monkeypatch.setattr("app.config.settings.prompt_version", "v999-missing")
    chunks = [
        {"text": "X is a thing.", "filename": "f.pdf", "page_number": 1},
    ]
    with pytest.raises(KeyError):
        build_rag_prompt(question="q", chunks=chunks)


def test_settings_validate_rejects_unknown_prompt_version():
    from app.config import Settings

    s = Settings(prompt_version="v999-missing")
    with pytest.raises(ValueError, match="prompt_version"):
        s.validate()
