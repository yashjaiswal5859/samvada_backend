from app.services.tts.base import BaseTTS
from app.services.tts.openai_tts import OpenAITTS
from app.services.tts.elevenlabs_tts import ElevenLabsTTS
from app.services.tts.cartesia_tts import CartesiaTTS

class TTSFactory:
    @staticmethod
    def get_tts(provider: str) -> BaseTTS:
        provider_name = provider.split(":")[0].strip().lower() if provider else "openai"
        if provider_name == "openai":
            return OpenAITTS()
        elif provider_name == "elevenlabs":
            return ElevenLabsTTS()
        elif provider_name == "cartesia":
            return CartesiaTTS()
        else:
            raise ValueError(f"Unsupported TTS provider: {provider}")
