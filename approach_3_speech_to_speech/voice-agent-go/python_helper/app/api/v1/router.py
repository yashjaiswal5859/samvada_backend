from fastapi import APIRouter

from .auth import router as auth_router
from .tenants import router as tenants_router
from .users import router as users_router
from .calls import router as calls_router
from .knowledge import router as knowledge_router
from .analytics import router as analytics_router
from .health import router as health_router

api_router = APIRouter()

api_router.include_router(auth_router)
api_router.include_router(tenants_router)
api_router.include_router(users_router)
api_router.include_router(calls_router)
api_router.include_router(knowledge_router)
api_router.include_router(analytics_router)
api_router.include_router(health_router)
