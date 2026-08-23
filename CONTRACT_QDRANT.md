# CONTRACT_QDRANT.md — Qdrant Payload Schema

This file defines the exact payload field names and values that the
**document pipeline (Teammate 2)** MUST write during ingestion and that
the **retriever (this service)** reads during search.

> ⚠️ If Teammate 2 uses different field names, the retriever will return
> empty strings for every chunk. The synthesizer will then see no text
> and set all domains to `insufficient_evidence`. The pipeline will not
> error — it will silently produce a low-quality roadmap. **Agree on
> these field names in the first 30 minutes.**

---

## Required Payload Fields

Every document chunk upserted into the `ipsakti_docs` collection MUST
include all of the following fields:

| Field        | Type   | Example value                             | Notes                                       |
|-------------|--------|-------------------------------------------|---------------------------------------------|
| `text`       | string | `"Section 3(p) of the Patents Act 1970 states..."` | The full chunk text. Must be non-empty. |
| `doc_title`  | string | `"Patents Act 1970"`                       | Human-readable document name.               |
| `section`    | string | `"Section 3(p)"`                           | Document section/clause. Empty string `""` if N/A. |
| `domain`     | string | `"patent"`                                 | See **Domain Values** below.                |
| `jurisdiction` | string | `"india"`                               | Always lowercase. `"india"` for all MVP docs. |
| `authority`  | string | `"statute"`                                | See **Authority Values** below.             |
| `source_url` | string | `"https://ipindia.gov.in/patents-act.htm"` | Official source URL. Empty string if N/A.   |
| `retrieved_at` | string | `"2026-08-22"`                          | ISO 8601 date the doc was fetched/ingested. |

---

## Domain Values

The `domain` field MUST be one of these exact strings:

| Value                  | Description                                          |
|------------------------|------------------------------------------------------|
| `patent`               | Patent law, prior art, patentability                 |
| `traditional_knowledge` | TKDL, classical texts, TK disclosure                |
| `biodiversity_abs`     | Biological Diversity Act, NBA, ABS compliance        |
| `regulatory`           | AYUSH licensing, Drugs & Cosmetics Act               |
| `trademark`            | Trade Marks Act, brand registration                  |

---

## Authority Values

The `authority` field MUST be one of these exact strings (case-insensitive
at read time, but use lowercase for consistency):

| Value       | AuthorityLevel | Description                                     |
|-------------|---------------|--------------------------------------------------|
| `statute`   | 4             | Acts of Parliament, primary legislation          |
| `rules`     | 3             | Statutory rules, regulations                     |
| `guidance`  | 2             | Government guidance documents, official FAQs     |
| `secondary` | 1             | Legal blogs, academic papers, commentary         |

---

## Qdrant Collection Details

- **Collection name**: `ipsakti_docs`
- **Vector model**: `text-embedding-004` (Google AI)
- **Embedding task type during ingestion**: `RETRIEVAL_DOCUMENT`
- **Embedding task type during retrieval**: `RETRIEVAL_QUERY`
- **Vector dimensions**: 768
- **Distance metric**: Cosine

> ⚠️ The embedding model and task type at retrieval time MUST match the
> model used during ingestion. If you switch models, you MUST re-ingest
> all documents.

---

## Recommended Payload Indexes

For query performance, create payload indexes on the following fields:

```
POST /collections/ipsakti_docs/index
{ "field_name": "domain",       "field_schema": "keyword" }

POST /collections/ipsakti_docs/index
{ "field_name": "jurisdiction", "field_schema": "keyword" }
```

Without these indexes, Qdrant performs a full collection scan for every
filtered search, which degrades significantly as the corpus grows.

---

## Minimal Ingestion Example (Python/qdrant-client)

```python
from qdrant_client import QdrantClient
from qdrant_client.models import PointStruct

client = QdrantClient(host="localhost", port=6333)

client.upsert(
    collection_name="ipsakti_docs",
    points=[
        PointStruct(
            id="<uuid>",
            vector=embedding_vector,   # 768-dim float32, RETRIEVAL_DOCUMENT task
            payload={
                "text":         "Section 3(p) of the Patents Act 1970...",
                "doc_title":    "Patents Act 1970",
                "section":      "Section 3(p)",
                "domain":       "patent",
                "jurisdiction": "india",
                "authority":    "statute",
                "source_url":   "https://ipindia.gov.in/patents-act.htm",
                "retrieved_at": "2026-08-22",
            },
        )
    ],
)
```
