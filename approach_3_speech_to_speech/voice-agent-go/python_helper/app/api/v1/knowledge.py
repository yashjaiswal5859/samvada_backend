from fastapi import APIRouter, Depends, UploadFile, File, Form
from sqlalchemy.ext.asyncio import AsyncSession
import uuid
from app.schemas.knowledge import KnowledgeSourceResponse
from app.db.session import get_db

router = APIRouter(prefix="/knowledge", tags=["knowledge"])

@router.post("/upload", response_model=KnowledgeSourceResponse)
async def upload_knowledge(
    source_type: str = Form(...),
    tenant_id: uuid.UUID = Form(...),
    file: UploadFile = File(...),
    db: AsyncSession = Depends(get_db)
):
    # TODO: Call knowledge_service.ingest
    return None

@router.get("/sources", response_model=list[KnowledgeSourceResponse])
async def list_sources(tenant_id: uuid.UUID, db: AsyncSession = Depends(get_db)):
    # TODO: List from DB
    return []

@router.delete("/sources/{id}")
async def delete_source(id: uuid.UUID, db: AsyncSession = Depends(get_db)):
    # TODO: Call knowledge_service.delete
    return {"status": "deleted"}
