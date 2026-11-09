from app.services.tts.base import BaseTTS
from typing import AsyncGenerator
# from elevenlabs.client import AsyncElevenLabs
from app.config import settings

class ElevenLabsTTS(BaseTTS):
    def __init__(self):
        # self.client = AsyncElevenLabs(api_key=settings.ELEVENLABS_API_KEY)
        pass
        
    async def stream_audio(self, text: str, voice: str) -> AsyncGenerator[bytes, None]:
        # Placeholder implementation
        yield b""
