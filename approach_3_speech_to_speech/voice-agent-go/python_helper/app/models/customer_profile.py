import uuid
from sqlalchemy.orm import Mapped, mapped_column
from sqlalchemy.dialects.postgresql import UUID, JSONB
from sqlalchemy import String, Integer

from app.models.base import Base, TenantMixin, TimestampMixin

class CustomerProfile(Base, TenantMixin, TimestampMixin):
    __tablename__ = "customer_profiles"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    phone: Mapped[str] = mapped_column(String(50), nullable=True)
    email: Mapped[str] = mapped_column(String(255), nullable=True)
    name: Mapped[str] = mapped_column(String(255), nullable=True)
    company: Mapped[str] = mapped_column(String(255), nullable=True)
    crm_id: Mapped[str] = mapped_column(String(255), nullable=True)
    crm_source: Mapped[str] = mapped_column(String(50), nullable=True)  # salesforce / hubspot / zoho
    metadata_col: Mapped[dict] = mapped_column(JSONB, default=dict)
    call_count: Mapped[int] = mapped_column(Integer, default=0)
