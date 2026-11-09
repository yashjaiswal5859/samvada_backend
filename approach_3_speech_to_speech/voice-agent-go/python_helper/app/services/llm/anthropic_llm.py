from app.services.llm.base import BaseLLM
from app.config import settings
# from anthropic import AsyncAnthropic

class AnthropicLLM(BaseLLM):
    def __init__(self):
        # self.client = AsyncAnthropic(api_key=settings.ANTHROPIC_API_KEY)
        pass
        
    async def generate_response(self, messages: list[dict], model: str) -> str:
        # Placeholder for actual implementation
        return "Claude response (placeholder)"
