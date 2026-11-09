from fastapi import Request, Depends
from app.db.redis_client import get_redis
import redis.asyncio as redis
from app.core.exceptions import RateLimitError

class RateLimiter:
    def __init__(self, max_requests: int = 100, window: int = 60):
        self.max_requests = max_requests
        self.window = window

    async def __call__(self, request: Request, redis_client: redis.Redis = Depends(get_redis)):
        # Rate limit based on IP or Tenant ID
        identifier = getattr(request.state, 'tenant_id', request.client.host)
        key = f"ratelimit:{identifier}:{request.url.path}"
        
        current = await redis_client.get(key)
        if current and int(current) >= self.max_requests:
            raise RateLimitError("Too many requests, please try again later.")
        
        pipe = redis_client.pipeline()
        pipe.incr(key)
        pipe.expire(key, self.window, nx=True)
        await pipe.execute()
