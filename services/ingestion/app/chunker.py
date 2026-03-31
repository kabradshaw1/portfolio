from langchain_text_splitters import RecursiveCharacterTextSplitter


def chunk_pages(
    pages: list[dict],
    chunk_size: int = 1000,
    chunk_overlap: int = 200,
) -> list[dict]:
    """Split page text into overlapping chunks.

    Returns list of dicts with 'text', 'page_number', and 'chunk_index' keys.
    Empty pages are skipped.
    """
    splitter = RecursiveCharacterTextSplitter(
        chunk_size=chunk_size,
        chunk_overlap=chunk_overlap,
        length_function=len,
    )

    chunks = []
    index = 0
    for page in pages:
        text = page["text"].strip()
        if not text:
            continue

        splits = splitter.split_text(text)
        for split in splits:
            chunks.append({
                "text": split,
                "page_number": page["page_number"],
                "chunk_index": index,
            })
            index += 1

    return chunks
