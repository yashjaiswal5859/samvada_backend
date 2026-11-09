from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy.ext.asyncio import AsyncSession
from app.schemas.auth import LoginRequest, TokenResponse, APIKeyCreate, APIKeyResponse
from app.core.security import create_access_token, generate_api_key
from app.db.session import get_db

router = APIRouter(prefix="/auth", tags=["auth"])

@router.post("/token", response_model=TokenResponse)
async def login(login_data: LoginRequest, db: AsyncSession = Depends(get_db)):
    # TODO: Validate user against DB using email & verify_password(login_data.password)
    # Placeholder login
    if login_data.email == "test@example.com" and login_data.password == "test":
        access_token = create_access_token(data={"sub": "user_id_123", "role": "admin"})
        return {"access_token": access_token, "token_type": "bearer"}
    
    raise HTTPException(
        status_code=status.HTTP_401_UNAUTHORIZED,
        detail="Incorrect email or password",
        headers={"WWW-Authenticate": "Bearer"},
    )

@router.post("/api-keys", response_model=APIKeyResponse)
async def create_api_key(api_key_data: APIKeyCreate, db: AsyncSession = Depends(get_db)):
    # TODO: Requires admin role, store hashed key in DB
    raw_key = generate_api_key()
    # return APIKeyResponse showing raw_key once
    return None
