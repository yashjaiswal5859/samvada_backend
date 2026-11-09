from app.services.stt.base import BaseSTT
from app.services.stt.deepgram_stt import DeepgramSTT
from app.services.stt.whisper_stt import WhisperSTT
from typing import Callable

class STTFactory:
    @staticmethod
    def get_stt(provider: str, callback: Callable) -> BaseSTT:
        provider_name = provider.split(":")[0].strip().lower() if provider else "deepgram"
        if provider_name == "deepgram":
            return DeepgramSTT(callback)
        elif provider_name == "whisper":
            return WhisperSTT(callback)
        else:
            raise ValueError(f"Unsupported STT provider: {provider}")
