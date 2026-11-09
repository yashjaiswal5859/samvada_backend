import uuid
from datetime import datetime
from sqlalchemy.orm import Mapped, mapped_column
from sqlalchemy.dialects.postgresql import UUID
from sqlalchemy import String, DateTime, func, Integer

from app.models.base import Base, TenantMixin

class Call(Base, TenantMixin):
    __tablename__ = "calls"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    session_id: Mapped[str] = mapped_column(String(255), unique=True, nullable=False, index=True)
    customer_phone: Mapped[str] = mapped_column(String(50), nullable=True)
    direction: Mapped[str] = mapped_column(String(10), nullable=True)  # inbound / outbound
    status: Mapped[str] = mapped_column(String(20), nullable=True)     # active / completed / escalated
    started_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=True)
    ended_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=True)
    duration_sec: Mapped[int] = mapped_column(Integer, nullable=True)
    outcome: Mapped[str] = mapped_column(String(50), nullable=True)
    audio_url: Mapped[str] = mapped_column(String(1024), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now(), nullable=False)
