from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy.ext.asyncio import AsyncSession
import uuid
from app.schemas.user import UserCreate, UserResponse
from app.db.session import get_db
from app.core.security import require_role, get_current_user

router = APIRouter(prefix="/users", tags=["users"])

@router.post("", response_model=UserResponse)
async def create_user(user: UserCreate, db: AsyncSession = Depends(get_db), current_user: dict = Depends(require_role(["admin"]))):
    # TODO: implementation
    return None

@router.get("", response_model=list[UserResponse])
async def list_users(db: AsyncSession = Depends(get_db), current_user: dict = Depends(get_current_user)):
    # TODO: implementation
    return []

@router.get("/{id}", response_model=UserResponse)
async def get_user(id: uuid.UUID, db: AsyncSession = Depends(get_db), current_user: dict = Depends(get_current_user)):
    # TODO: implementation
    return None

@router.put("/{id}")
async def update_user_role(id: uuid.UUID, role: str, db: AsyncSession = Depends(get_db), current_user: dict = Depends(require_role(["admin"]))):
    # TODO: implementation
    return {"status": "updated"}
