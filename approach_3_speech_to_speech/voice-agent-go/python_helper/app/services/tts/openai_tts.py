from app.services.tts.base import BaseTTS
from openai import AsyncOpenAI
from app.config import settings
import base64
from typing import AsyncGenerator

class OpenAITTS(BaseTTS):
    def __init__(self):
        self.client = AsyncOpenAI(api_key=settings.OPENAI_API_KEY)
        
    async def stream_audio(self, text: str, voice: str = "nova") -> AsyncGenerator[bytes, None]:
        response = await self.client.audio.speech.create(
            model="tts-1",
            voice=voice,
            input=text,
            response_format="pcm"
        )
        async for chunk in response.iter_bytes(chunk_size=1024):
            if chunk:
                # Assuming chunk is PCM and we encode to b64 for WebSocket
                yield base64.b64encode(chunk).decode("utf-8")
