from app.services.llm.base import BaseLLM
from app.config import settings
# import google.generativeai as genai

class GeminiLLM(BaseLLM):
    def __init__(self):
        # genai.configure(api_key=settings.GOOGLE_API_KEY)
        pass
        
    async def generate_response(self, messages: list[dict], model: str) -> str:
        # Placeholder for actual implementation
        return "Gemini response (placeholder)"
