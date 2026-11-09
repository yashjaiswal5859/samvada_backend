from fastapi import APIRouter, Depends, HTTPException, BackgroundTasks
from pydantic import BaseModel
import uuid
from typing import List, Optional
from sqlalchemy.future import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.db.session import get_db
from app.models.customer_memory import CustomerMemory
from app.services.rag.rag_manager import rag_manager
from app.services.knowledge_ingestion import KnowledgeIngestionService
from app.schemas.rag import RAGQuery, RAGResult

router = APIRouter(prefix="/rag/session", tags=["RAG Session v2"])

class RAGSessionInitRequest(BaseModel):
    session_id: str
    company_name: str
    tenant_id: uuid.UUID
    phone: Optional[str] = None

class RAGSessionSearchRequest(BaseModel):
    session_id: str
    query: str
    tenant_id: uuid.UUID

class RAGSessionIndexRequest(BaseModel):
    session_id: str
    text: str
    source: str
    tenant_id: uuid.UUID

class RAGSessionPersistRequest(BaseModel):
    session_id: str
    phone: str
    tenant_id: uuid.UUID
    memories: List[str]
    source_call_id: uuid.UUID

@router.post("/initialize")
async def initialize_session_rag(req: RAGSessionInitRequest, db: AsyncSession = Depends(get_db)):
    """
    Initializes a session-specific RAG vector index.
    1. Loads static company knowledge documents from disk.
    2. Loads customer historical memories from Postgres and ingests them.
    All data is kept isolated in memory under the session ID key.
    """
    namespace = f"session:{req.session_id}"
    
    # 1. Ingest static docs
    ingestion_service = KnowledgeIngestionService()
    try:
        await ingestion_service.ingest_company_docs(req.company_name, str(req.tenant_id), namespace=namespace)
    except FileNotFoundError:
        # It's fine if the company docs don't exist yet, we still create the memory index
        print(f"[RAG Session] Static folder docs/{req.company_name} not found. Starting with empty/memory-only index.")

    # 2. Ingest Postgres customer memories if phone number is provided
    if req.phone:
        query_result = await db.execute(
            select(CustomerMemory.memory_text).where(
                CustomerMemory.phone == req.phone,
                CustomerMemory.tenant_id == req.tenant_id
            )
        )
        memories = query_result.scalars().all()
        if memories:
            rag_index = rag_manager.get_index(namespace)
            docs_to_index = [{"text": mem, "source": "postgres_historical_memory"} for mem in memories]
            await rag_index.index_documents(docs_to_index, str(req.tenant_id))
            print(f"[RAG Session] Pre-loaded {len(memories)} historical memories for phone {req.phone} into {namespace}.")

    return {"status": "initialized", "session_id": req.session_id}

@router.post("/search", response_model=List[RAGResult])
async def search_session_rag(req: RAGSessionSearchRequest):
    """
    Searches the isolated session-specific index.
    """
    namespace = f"session:{req.session_id}"
    rag_index = rag_manager.get_index(namespace)
    query_obj = RAGQuery(query=req.query, tenant_id=req.tenant_id)
    results = await rag_index.search(query_obj)
    return results

@router.post("/index")
async def index_session_rag(req: RAGSessionIndexRequest):
    """
    Indexes a dynamic memory context directly into the live session index in RAM.
    """
    namespace = f"session:{req.session_id}"
    rag_index = rag_manager.get_index(namespace)
    await rag_index.index_documents([{"text": req.text, "source": req.source}], str(req.tenant_id))
    return {"status": "indexed", "session_id": req.session_id}

@router.post("/persist")
async def persist_session_memories(req: RAGSessionPersistRequest, db: AsyncSession = Depends(get_db)):
    """
    Saves a list of newly accumulated dynamic memories into Postgres for future calls.
    """
    for memory_text in req.memories:
        memory_obj = CustomerMemory(
            tenant_id=req.tenant_id,
            phone=req.phone,
            memory_text=memory_text,
            source_call_id=req.source_call_id
        )
        db.add(memory_obj)
    await db.commit()
    return {"status": "persisted", "memories_saved": len(req.memories)}

@router.delete("/{session_id}")
async def destroy_session_rag(session_id: str):
    """
    Drops the isolated session vector index from RAM completely.
    """
    namespace = f"session:{session_id}"
    rag_manager.drop_index(namespace)
    return {"status": "destroyed", "session_id": session_id}
