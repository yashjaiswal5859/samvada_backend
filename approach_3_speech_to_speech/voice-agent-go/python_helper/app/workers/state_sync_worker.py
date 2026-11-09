import asyncio
import json
from aiokafka import AIOKafkaConsumer
from app.config import settings
from app.db.session import async_session_maker
from app.db.redis_client import redis_pool
import redis.asyncio as redis
from app.services.conversation_state import get_full_state
from app.models.conversation_history import ConversationHistory
from app.models.emotion_timeline import EmotionTimeline

class StateSyncWorker:
    def __init__(self):
        self.consumer = AIOKafkaConsumer(
            "call.lifecycle",
            bootstrap_servers=settings.KAFKA_BOOTSTRAP_SERVERS,
            group_id="state_sync_group"
        )
        
    async def start(self):
        await self.consumer.start()
        try:
            async for msg in self.consumer:
                data = json.loads(msg.value.decode("utf-8"))
                if data.get("event") == "ended":
                    session_id = data.get("session_id")
                    await self.archive_session(session_id)
        finally:
            await self.consumer.stop()
            
    async def archive_session(self, session_id: str):
        redis_client = redis.Redis(connection_pool=redis_pool)
        
        # 1. Read full state from Redis before it expires
        state = await get_full_state(session_id, redis_client)
        
        # 2. Persist to Postgres
        async with async_session_maker() as db:
            # We would look up Call ID here based on session_id
            # This is simplified for brevity
            print(f"Archiving session {session_id} to DB. {state.turn_count} turns recorded.")
            
            # Example logic for DB persistence
            # db.add_all([ConversationHistory(call_id=..., role=t.role, content=t.content, timestamp=t.timestamp) for t in state.transcript])
            # if state.emotion_timeline:
            #     db.add(EmotionTimeline(call_id=..., timeline=[e.model_dump() for e in state.emotion_timeline.timeline]))
            
            await db.commit()
            
        await redis_client.aclose()

if __name__ == "__main__":
    worker = StateSyncWorker()
    asyncio.run(worker.start())
