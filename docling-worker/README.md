# Samvada Docling Document Ingestion Worker

[![Python Version](https://img.shields.io/badge/Python-3.11-blue?style=flat&logo=python)](https://python.org/)
[![pgvector](https://img.shields.io/badge/PostgreSQL-pgvector-336791?style=flat&logo=postgresql)](https://github.com/pgvector/pgvector)
[![MinIO](https://img.shields.io/badge/MinIO-S3_Storage-C72C48?style=flat&logo=minio)](https://min.io/)

> **Asynchronous Document Parsing & RAG Embedding Pipeline**  
> Consumes uploaded knowledge-base documents from MinIO S3, extracts structured text via IBM Docling, and computes dense semantic vector embeddings stored in PostgreSQL (`pgvector`) for Agent contextual retrieval.

---

## 🔄 Ingestion Pipeline Flow

```
┌─────────────────┐       ┌─────────────────┐       ┌──────────────────┐
│  ABS / Upload   │──────▶│    MinIO S3     │       │   Kafka Topic    │
│   (:8082 API)   │       │ (Raw PDF/DOCX)  │       │ (kb.doc.ingest)  │
└─────────────────┘       └────────┬────────┘       └────────┬─────────┘
                                   │                         │
                                   ▼                         ▼
                          ┌────────────────────────────────────────────┐
                          │            Docling Worker                  │
                          │ 1. Download document from MinIO            │
                          │ 2. Parse layout & tables (IBM Docling)     │
                          │ 3. Chunk text semantically                 │
                          │ 4. Embed chunks (sentence-transformers)    │
                          └─────────────────────┬──────────────────────┘
                                                │
                                                ▼
                                  ┌───────────────────────────┐
                                  │ PostgreSQL (pgvector)     │
                                  │ (samvada_abs.kb_chunks)   │
                                  └───────────────────────────┘
```

---

## ⚙️ Configuration & Environment

| Variable | Default | Purpose |
|---|---|---|
| `KAFKA_BROKERS` | `localhost:9092` | Kafka cluster bootstrap servers |
| `KAFKA_GROUP_ID` | `docling-worker-group` | Consumer group ID |
| `MINIO_ENDPOINT` | `localhost:9000` | S3 storage endpoint |
| `POSTGRES_DSN` | `postgres://user:pass@localhost:5432/samvada_abs` | Vector storage database connection |
| `EMBEDDING_MODEL`| `all-MiniLM-L6-v2` | Sentence-transformers model name |

---

## 🚀 Running Locally

```bash
# Install dependencies
pip install -r requirements.txt

# Run worker
python worker.py
```

### Docker
```bash
docker build -t samvada-docling-worker .
docker run --env-file .env samvada-docling-worker
```

