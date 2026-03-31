SYSTEM_PROMPT = """You are a helpful document Q&A assistant. Answer questions based only on the provided context. If the context doesn't contain enough information to answer, say so honestly — do not make up information.

When referencing information, mention the source file and page number."""

RAG_TEMPLATE = """Context:
{context}

Question: {question}

Answer based only on the context above. Cite sources (filename, page) when possible."""

NO_CONTEXT_TEMPLATE = """The user asked: {question}

I don't have any relevant context from uploaded documents to answer this question. Please upload a relevant document first, or rephrase your question."""


def build_rag_prompt(question: str, chunks: list[dict]) -> str:
    if not chunks:
        return NO_CONTEXT_TEMPLATE.format(question=question)

    context_parts = []
    for chunk in chunks:
        source = f"[{chunk['filename']}, page {chunk['page_number']}]"
        context_parts.append(f"{source}\n{chunk['text']}")

    context = "\n\n".join(context_parts)
    return RAG_TEMPLATE.format(context=context, question=question)
