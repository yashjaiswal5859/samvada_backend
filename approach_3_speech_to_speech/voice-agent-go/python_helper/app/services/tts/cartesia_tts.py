from app.services.tts.base import BaseTTS
from app.config import settings
import base64
import json
import httpx
from typing import AsyncGenerator

class CartesiaTTS(BaseTTS):
    def __init__(self):
        self.api_key = settings.CARTESIA_API_KEY
        # Cartesia Sonic Indian Female Accent Voice ID
        self.default_voice = "a0e09861-5728-478e-b408-8c927743f9f4"
        
    async def stream_audio(self, text: str, voice: str = None) -> AsyncGenerator[bytes, None]:
        voice_id = voice if voice else self.default_voice
        headers = {
            "X-API-Key": self.api_key,
            "Cartesia-Version": "2024-06-10",
            "Content-Type": "application/json"
        }
        payload = {
            "model_id": "sonic-multilingual",
            "transcript": text,
            "voice": {
                "mode": "id",
                "id": voice_id
            },
            "output_format": {
                "container": "raw",
                "encoding": "pcm_mulaw",
                "sample_rate": 8000
            }
        }
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.post("https://api.cartesia.ai/tts/bytes", headers=headers, json=payload)
            if resp.status_code == 200:
                audio_b64 = base64.b64encode(resp.content).decode("utf-8")
                yield audio_b64
