from fastapi import Request
from fastapi.responses import JSONResponse

class BaseAPIError(Exception):
    def __init__(self, message: str, status_code: int = 500):
        self.message = message
        self.status_code = status_code

class NotFoundError(BaseAPIError):
    def __init__(self, message: str = "Resource not found"):
        super().__init__(message, 404)

class UnauthorizedError(BaseAPIError):
    def __init__(self, message: str = "Unauthorized"):
        super().__init__(message, 401)

class ForbiddenError(BaseAPIError):
    def __init__(self, message: str = "Forbidden"):
        super().__init__(message, 403)

class RateLimitError(BaseAPIError):
    def __init__(self, message: str = "Rate limit exceeded"):
        super().__init__(message, 429)

class ExternalServiceError(BaseAPIError):
    def __init__(self, message: str = "External service failed"):
        super().__init__(message, 502)

async def api_error_handler(request: Request, exc: BaseAPIError):
    return JSONResponse(
        status_code=exc.status_code,
        content={"error": exc.__class__.__name__, "detail": exc.message},
    )

def setup_exception_handlers(app):
    app.add_exception_handler(BaseAPIError, api_error_handler)
    # can add more specific handlers here
