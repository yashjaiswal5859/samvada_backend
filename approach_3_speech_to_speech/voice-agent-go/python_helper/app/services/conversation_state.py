import json
import redis.asyncio as redis
from app.schemas.conversation import ConversationState, TranscriptEntry
from app.schemas.emotion import EmotionEntry, EmotionState
from app.schemas.strategy import StrategyOutput
from app.schemas.tool import ToolCallResult
from datetime import datetime

# Write operations
async def append_transcript(session_id: str, entry: TranscriptEntry, redis_client: redis.Redis):
    key = f"session:{session_id}:transcript"
    await redis_client.rpush(key, entry.model_dump_json())
    await redis_client.hset(f"session:{session_id}:state", "last_activity_at", datetime.utcnow().isoformat())

async def update_emotion_timeline(session_id: str, entry: EmotionEntry, redis_client: redis.Redis):
    key = f"session:{session_id}:emotion"
    await redis_client.rpush(key, entry.model_dump_json())

async def update_strategy(session_id: str, strategy: StrategyOutput, redis_client: redis.Redis):
    key = f"session:{session_id}:strategy"
    await redis_client.set(key, strategy.model_dump_json())

async def set_memory(session_id: str, mem_key: str, value: str, redis_client: redis.Redis):
    key = f"session:{session_id}:memory"
    await redis_client.hset(key, mem_key, value)

async def append_intent(session_id: str, intent: str, redis_client: redis.Redis):
    key = f"session:{session_id}:intent_history"
    await redis_client.rpush(key, intent)

async def append_tool_result(session_id: str, result: ToolCallResult, redis_client: redis.Redis):
    key = f"session:{session_id}:tool_history"
    await redis_client.rpush(key, result.model_dump_json())

async def increment_turn_count(session_id: str, redis_client: redis.Redis):
    key = f"session:{session_id}:state"
    await redis_client.hincrby(key, "turn_count", 1)

# Read operations (Pipeline)
async def get_full_state(session_id: str, redis_client: redis.Redis) -> ConversationState:
    pipe = redis_client.pipeline()
    
    pipe.hgetall(f"session:{session_id}:state")
    pipe.lrange(f"session:{session_id}:transcript", 0, -1)
    pipe.lrange(f"session:{session_id}:emotion", 0, -1)
    pipe.get(f"session:{session_id}:strategy")
    pipe.hgetall(f"session:{session_id}:memory")
    pipe.lrange(f"session:{session_id}:intent_history", 0, -1)
    pipe.lrange(f"session:{session_id}:tool_history", 0, -1)
    
    results = await pipe.execute()
    
    state_hash = results[0] or {}
    transcript_raw = results[1] or []
    emotion_raw = results[2] or []
    strategy_raw = results[3]
    memory_hash = results[4] or {}
    intent_list = results[5] or []
    tool_raw = results[6] or []

    # Parse
    transcript = [TranscriptEntry.model_validate_json(t) for t in transcript_raw]
    timeline = [EmotionEntry.model_validate_json(e) for e in emotion_raw]
    
    emotion_state = None
    if timeline:
        # Simple parsing for now
        current = timeline[-1]
        previous = timeline[-2] if len(timeline) > 1 else None
        emotion_state = EmotionState(
            timeline=timeline,
            current_emotion=current.emotion,
            previous_emotion=previous.emotion if previous else None,
            emotion_confidence=current.confidence,
            emotion_trend="stable",
            emotion_velocity=0.0
        )
    
    strategy = StrategyOutput.model_validate_json(strategy_raw) if strategy_raw else None
    tool_history = [ToolCallResult.model_validate_json(t) for t in tool_raw]

    return ConversationState(
        session_id=session_id,
        customer_phone=state_hash.get("customer_phone"),
        transcript=transcript,
        emotion_timeline=emotion_state,
        strategy=strategy,
        memory=memory_hash,
        intent_history=intent_list,
        tool_history=tool_history,
        turn_count=int(state_hash.get("turn_count", 0))
    )
