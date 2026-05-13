# RAG Product Document Seed Job

- **Date:** 2026-05-13
- **Status:** Accepted

## Context

The portfolio demo needs product-catalog PDFs available in the RAG vector
database for QA and production. The PDFs live in the repository under
`docs/product-catalog/`, but the Debian host running QA and production does not
maintain a checked-out copy of the repository as runtime input.

The first seeding approach used a shell script that posted PDFs to the
ingestion HTTP endpoint. That approach had several operational problems:

- It required the PDF files to exist wherever the script ran.
- It depended on public or ingress routing to the ingestion service.
- It had to satisfy ingestion JWT authentication.
- It was constrained by the public ingestion API rate limit.
- It was non-fatal in deploy scripts, so seed failures could be missed.

QA and production share the same Qdrant instance in this portfolio deployment,
but use separate default collections: `documents_qa` for QA and `documents` for
production. Any seeding mechanism must preserve that separation and avoid
adding duplicate chunks on every deploy.

## Decision

Seed product-catalog PDFs with a committed Kubernetes Job that runs inside the
cluster and calls ingestion internals directly, rather than posting to the
ingestion HTTP API.

The ingestion image packages the product-catalog PDFs at
`/app/seed/product-catalog/`. The seed command reuses the existing ingestion
pipeline components:

- PDF parsing
- page chunking
- dense embedding generation
- sparse vector generation
- Qdrant upsert logic
- collection metadata persistence

Deploy pipelines delete, apply, and wait for the `seed-product-docs` Job after
the ingestion deployment is available. The Job reads the same `ingestion-config`
as the service, so QA seeds `documents_qa` and production seeds `documents`.

The seed runner is idempotent at the collection level. It checks the target
collection point count before doing work. If the collection already contains
points, it exits without parsing PDFs or upserting chunks. This prevents
duplicates when CI/CD reruns the Job on every deploy.

## Consequences

Product demo data is now seeded through deployable, auditable code instead of a
manual or best-effort script. The seed path avoids public HTTP auth and rate
limits while still using the same parser, chunker, embedding, sparse-vector,
and Qdrant storage behavior as the ingestion service.

Seeding now fails the deploy if the Job cannot populate an empty collection.
That is intentional: a successful deploy should leave the demo RAG corpus
usable.

The main trade-off is that this is not an automatic content synchronization
system. If product PDFs, chunking settings, or embedding models change, an
already-populated collection will not be updated by default. A future controlled
reseed mechanism should use an explicit flag, version marker, or delete/recreate
workflow rather than silently appending duplicate data.

Packaging the PDFs in the ingestion image slightly couples demo fixture content
to that image. The PDF set is currently small, and this keeps the deploy path
simple. If seed datasets grow substantially or become environment-specific, a
dedicated seed image or artifact-backed seed process would be a better fit.
