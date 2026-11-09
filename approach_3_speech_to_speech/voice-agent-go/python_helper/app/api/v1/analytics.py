from fastapi import APIRouter, Depends
from sqlalchemy.ext.asyncio import AsyncSession
import uuid
from app.schemas.analytics import AnalyticsSummary
from app.db.session import get_db

router = APIRouter(prefix="/analytics", tags=["analytics"])

@router.get("/summary", response_model=AnalyticsSummary)
async def get_summary(tenant_id: uuid.UUID, db: AsyncSession = Depends(get_db)):
    # TODO: Aggregate metrics from PostgreSQL
    return {
        "total_calls": 0,
        "avg_duration_sec": 0.0,
        "conversion_rate": 0.0,
        "top_outcomes": {},
        "emotion_distribution": {}
    }
