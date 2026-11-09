from app.services.vad.base import BaseVAD
from app.services.vad.silero_vad import SileroVAD
from app.services.vad.webrtc_vad import WebRTCVAD

class VADFactory:
    @staticmethod
    def get_vad(provider: str = "silero") -> BaseVAD:
        provider_name = provider.split(":")[0].strip().lower() if provider else "silero"
        if provider_name == "silero":
            return SileroVAD()
        elif provider_name == "webrtc":
            return WebRTCVAD()
        else:
            raise ValueError(f"Unsupported VAD provider: {provider}")
