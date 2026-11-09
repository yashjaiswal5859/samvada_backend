import uuid
from sqlalchemy.orm import Mapped, mapped_column
from sqlalchemy.dialects.postgresql import UUID
from sqlalchemy import String, Text

from app.models.base import Base, TenantMixin, TimestampMixin

class CustomerMemory(Base, TenantMixin, TimestampMixin):
    __tablename__ = "customer_memories"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    phone: Mapped[str] = mapped_column(String(50), nullable=False, index=True)
    memory_text: Mapped[str] = mapped_column(Text, nullable=False)
    source_call_id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), nullable=False, index=True)
