from app.services.llm.base import BaseLLM
from openai import AsyncOpenAI
from app.config import settings

class OpenAILLM(BaseLLM):
    def __init__(self):
        self.client = AsyncOpenAI(api_key=settings.OPENAI_API_KEY)
        
    async def generate_response(self, messages: list[dict], model: str) -> str:
        response = await self.client.chat.completions.create(
            model=model,
            messages=messages,
            temperature=0.7,
            max_tokens=150
        )
        return response.choices[0].message.content
