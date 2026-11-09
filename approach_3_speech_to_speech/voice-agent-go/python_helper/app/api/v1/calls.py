from fastapi import APIRouter, Depends, HTTPException, BackgroundTasks
from sqlalchemy.ext.asyncio import AsyncSession
import uuid
import json
import asyncio
import pandas as pd
from pathlib import Path
import redis.asyncio as redis

from app.schemas.call import CallResponse, OutboundCallRequest, BatchCampaignRequest
from app.db.session import get_db
from app.db.redis_client import redis_pool
from app.services.knowledge_ingestion import KnowledgeIngestionService
from app.services.rag.rag_manager import rag_manager

router = APIRouter(prefix="/calls", tags=["calls"])

def process_outbound_batch(req: BatchCampaignRequest, tenant_id: str):
    """Background task to ingest docs and make calls with custom providers."""
    try:
        # 1. Ingest Docs into an ISOLATED namespace for this company
        ingestion_service = KnowledgeIngestionService()
        
        loop = asyncio.new_event_loop()
        asyncio.set_event_loop(loop)
        doc_count = loop.run_until_complete(ingestion_service.ingest_company_docs(req.company_name, tenant_id))
        
        # 2. Parse Leads
        leads_path = Path("D:/Voice AI/docs") / req.company_name / "leads.csv"
        if not leads_path.exists():
            print(f"No leads.csv found in D:/Voice AI/docs/{req.company_name}")
            return
            
        df = pd.read_csv(leads_path)
        
        # Redis client to save the call configuration for the dynamic websocket flow
        redis_client = redis.Redis(connection_pool=redis_pool)
        
        # 3. Trigger Calls
        for index, row in df.iterrows():
            phone = row.get("phone")
            name = row.get("name")
            if phone:
                call_sid = f"twilio_{uuid.uuid4()}"
                
                # Store the custom routing flow configuration in Redis mapped by call_sid
                # CRITICAL: rag_namespace ensures the WebSocket loads the CORRECT isolated index
                config_key = f"call_config:{call_sid}"
                config_data = {
                    "stt_provider": req.stt_provider,
                    "llm_model": req.llm_model,
                    "tts_provider": req.tts_provider,
                    "vad_provider": req.vad_provider,
                    "audio_emotion_provider": req.audio_emotion_provider,
                    "text_emotion_provider": req.text_emotion_provider,
                    "rag_namespace": req.company_name,
                    "tenant_id": tenant_id
                }
                
                loop.run_until_complete(redis_client.set(config_key, json.dumps(config_data), ex=3600)) # Expire in 1 hour
                
                # Mock Twilio API call
                print(
                    f"[Outbound Campaign] Dialing {name} at {phone} for {req.company_name}... "
                    f"(VAD: {req.vad_provider}, STT: {req.stt_provider}, LLM: {req.llm_model}, "
                    f"TTS: {req.tts_provider}, AudioEmotion: {req.audio_emotion_provider}, "
                    f"TextEmotion: {req.text_emotion_provider}, RAG namespace: {req.company_name} - {doc_count} docs ingested)"
                )
        
        loop.run_until_complete(redis_client.aclose())
                
    except Exception as e:
        print(f"Batch Error: {e}")

@router.post("/batch")
async def trigger_batch_campaign(req: BatchCampaignRequest, background_tasks: BackgroundTasks):
    """
    Triggers a batch outbound campaign for a company, configuring STT, LLM, TTS dynamically.
    If optional models are omitted, low-cost defaults are automatically applied.
    """
    tenant_id = str(uuid.uuid4()) # Mock tenant lookup
    background_tasks.add_task(process_outbound_batch, req, tenant_id)
    
    return {
        "status": "accepted", 
        "message": f"Campaign started for {req.company_name}.",
        "configuration": req.model_dump()
    }

@router.post("/outbound", response_model=CallResponse)
async def initiate_outbound_call(request: OutboundCallRequest, db: AsyncSession = Depends(get_db)):
    # TODO: Call Twilio API to initiate call
    return None

@router.get("", response_model=list[CallResponse])
async def list_calls(status: str = None, db: AsyncSession = Depends(get_db)):
    # TODO: Query DB for calls, filter by tenant_id
    return []

@router.get("/{id}", response_model=CallResponse)
async def get_call(id: uuid.UUID, db: AsyncSession = Depends(get_db)):
    # TODO: Fetch call by ID
    return None

@router.post("/{id}/escalate")
async def escalate_call(id: uuid.UUID, db: AsyncSession = Depends(get_db)):
    # TODO: Mark as escalated and notify human agent
    return {"status": "escalated"}
