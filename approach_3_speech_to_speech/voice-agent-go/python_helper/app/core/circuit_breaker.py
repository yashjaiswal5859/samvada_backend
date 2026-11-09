import pybreaker
from app.core.exceptions import ExternalServiceError

# Circuit breaker for external API calls (e.g., LLM, CRM)
# Fails after 5 consecutive errors, resets after 30 seconds
external_api_breaker = pybreaker.CircuitBreaker(
    fail_max=5,
    reset_timeout=30,
    state_storage=pybreaker.MemoryStorage()
)

def handle_circuit_breaker_error(func):
    """Decorator to catch circuit breaker exceptions and raise HTTP 502"""
    async def wrapper(*args, **kwargs):
        try:
            return await func(*args, **kwargs)
        except pybreaker.CircuitBreakerError as e:
            raise ExternalServiceError(f"Service temporarily unavailable: {str(e)}")
    return wrapper
