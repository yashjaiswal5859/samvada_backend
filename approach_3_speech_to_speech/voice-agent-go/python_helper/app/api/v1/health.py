from fastapi import APIRouter

router = APIRouter(tags=["health"])

@router.get("/ready")
async def readiness_check():
    # TODO: Check DB, Redis, Kafka ping
    return {"status": "ready"}
