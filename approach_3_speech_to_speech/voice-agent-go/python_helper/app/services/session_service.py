import uuid
import json
import redis.asyncio as redis
from sqlalchemy.ext.asyncio import AsyncSession
from datetime import datetime

from app.models.call import Call
from app.workers.kafka_producer import get_kafka_producer
from app.schemas.conversation import ConversationState, TranscriptEntry
from app.services.conversation_state import get_full_state

async def create_session(tenant_id: uuid.UUID, customer_phone: str, direction: str, redis_client: redis.Redis, db: AsyncSession) -> str:
    session_id = str(uuid.uuid4())
    
    # Init Redis hash
    state_key = f"session:{session_id}:state"
    await redis_client.hset(state_key, mapping={
        "customer_phone": customer_phone or "",
        "session_started_at": datetime.utcnow().isoformat(),
        "turn_count": 1,
        "last_activity_at": datetime.utcnow().isoformat()
    })
    
    # Pre-populate greeting to transcript in Redis
    greeting_entry = TranscriptEntry(
        role="assistant",
        content="Hi, anjali this side. Am I speaking with yash?",
        timestamp=datetime.utcnow(),
        is_final=True
    )
    await redis_client.rpush(f"session:{session_id}:transcript", greeting_entry.model_dump_json())
    
    # Create Call row in DB
    new_call = Call(
        tenant_id=tenant_id,
        session_id=session_id,
        customer_phone=customer_phone,
        direction=direction,
        status="active",
        started_at=datetime.utcnow()
    )
    db.add(new_call)
    await db.commit()
    
    # Publish to Kafka
    producer = await get_kafka_producer()
    await producer.publish(
        topic="call.lifecycle",
        key=session_id,
        value={"session_id": session_id, "event": "started", "tenant_id": str(tenant_id)}
    )
    
    return session_id

async def end_session(session_id: str, outcome: str, redis_client: redis.Redis, db: AsyncSession):
    state_key = f"session:{session_id}:state"
    await redis_client.hset(state_key, "status", "completed")
    
    # Publish lifecycle event
    producer = await get_kafka_producer()
    await producer.publish(
        topic="call.lifecycle",
        key=session_id,
        value={"session_id": session_id, "event": "ended", "outcome": outcome}
    )
    
    # Set TTL for Redis keys (expire in 5 minutes after call ends)
    pipe = redis_client.pipeline()
    pipe.expire(state_key, 300)
    pipe.expire(f"session:{session_id}:transcript", 300)
    pipe.expire(f"session:{session_id}:emotion", 300)
    pipe.expire(f"session:{session_id}:strategy", 300)
    pipe.expire(f"session:{session_id}:memory", 300)
    pipe.expire(f"session:{session_id}:intent_history", 300)
    pipe.expire(f"session:{session_id}:tool_history", 300)
    await pipe.execute()
    
    # Let the state_sync_worker handle archiving transcript & emotion timeline to DB
    # (Triggered by call.lifecycle ended event)
