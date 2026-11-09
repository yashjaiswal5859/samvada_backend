from fastapi import APIRouter, WebSocket, WebSocketDisconnect
import uuid
import json
import asyncio
from app.db.session import async_session_maker
from app.db.redis_client import redis_pool
import redis.asyncio as redis
from app.services.session_service import create_session, end_session
from app.config import settings

# Factories
from app.services.stt.factory import STTFactory
from app.services.llm.factory import LLMFactory
from app.services.tts.factory import TTSFactory
from app.services.vad.factory import VADFactory
from app.services.rag.rag_manager import rag_manager
from app.services.emotion.factory import AudioEmotionFactory, TextEmotionFactory

router = APIRouter(prefix="/ws", tags=["websocket"])

@router.websocket("/stream/{call_sid}")
async def audio_stream_endpoint(websocket: WebSocket, call_sid: str, tenant_id: str, phone: str = ""):
    await websocket.accept()
    
    redis_client = redis.Redis(connection_pool=redis_pool)
    session_id = None
    
    try:
        # 1. Fetch Call Campaign configuration from Redis (if this was an outbound campaign call)
        config_key = f"call_config:{call_sid}"
        config_raw = await redis_client.get(config_key)
        
        if config_raw:
            config = json.loads(config_raw)
            stt_provider = config.get("stt_provider", "deepgram")
            llm_model = config.get("llm_model", settings.DEFAULT_LLM_MODEL)
            tts_provider = config.get("tts_provider", "openai")
            vad_provider = config.get("vad_provider", "silero")
            audio_emotion_provider = config.get("audio_emotion_provider", "emotion2vec")
            text_emotion_provider = config.get("text_emotion_provider", "distilroberta")
            rag_namespace = config.get("rag_namespace", "default")
            actual_tenant_id = uuid.UUID(config.get("tenant_id"))
        else:
            # Low cost defaults for inbound calls
            stt_provider = "deepgram"
            llm_model = settings.DEFAULT_LLM_MODEL
            tts_provider = "openai"
            vad_provider = "silero"
            audio_emotion_provider = "emotion2vec"
            text_emotion_provider = "distilroberta"
            rag_namespace = "default"
            actual_tenant_id = uuid.UUID(tenant_id)
            
        # 2. Initialize Session
        async with async_session_maker() as db:
            session_id = await create_session(
                tenant_id=actual_tenant_id,
                customer_phone=phone,
                direction="outbound" if config_raw else "inbound",
                redis_client=redis_client,
                db=db
            )
        
        # 3. Instantiate dynamic processing flow
        async def on_transcript_chunk(text: str, is_final: bool):
            print(f"[{session_id}] Transcribed: {text} (is_final={is_final})")

        stt_engine = STTFactory.get_stt(stt_provider, on_transcript_chunk)
        llm_engine = LLMFactory.get_llm(llm_model)
        tts_engine = TTSFactory.get_tts(tts_provider)
        vad_engine = VADFactory.get_vad(vad_provider)
        rag_engine = rag_manager.get_index(rag_namespace)
        
        # Instantiate SOLID emotion classifiers
        audio_emotion_engine = AudioEmotionFactory.get_classifier(audio_emotion_provider)
        text_emotion_engine = TextEmotionFactory.get_classifier(text_emotion_provider)
        
        await stt_engine.connect()
        
        print(
            f"[{session_id}] WebSocket stream connected for call {call_sid}.\n"
            f"Active Flow -> STT: {stt_provider} | LLM: {llm_model} | TTS: {tts_provider} | VAD: {vad_provider} | "
            f"AudioEmotion: {audio_emotion_provider} | TextEmotion: {text_emotion_provider}"
        )
        
        while True:
            data = await websocket.receive_text()
            msg = json.loads(data)
            
            if msg["event"] == "start":
                print(f"[{session_id}] Stream started: {msg['start']['streamSid']}")
            elif msg["event"] == "media":
                payload = msg["media"]["payload"]
                # Decode and run VAD check
                # Example:
                # if vad_engine.process_audio(payload):
                #     # Handle user interruption (barge-in)
                await stt_engine.send_audio(payload.encode("utf-8"))
            elif msg["event"] == "stop":
                print(f"[{session_id}] Stream stopped")
                break
                
        await stt_engine.close()
                
    except WebSocketDisconnect:
        print(f"[{session_id}] WebSocket disconnected")
    except Exception as e:
        print(f"[{session_id}] Error in WebSocket: {e}")
    finally:
        if session_id:
            async with async_session_maker() as db:
                await end_session(session_id, outcome="completed", redis_client=redis_client, db=db)
        await redis_client.aclose()
