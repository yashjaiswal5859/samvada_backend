from app.services.stt.base import BaseSTT

class WhisperSTT(BaseSTT):
    def __init__(self, callback):
        super().__init__(callback)
        # self.client = ...
        
    async def connect(self):
        pass

    async def send_audio(self, audio_chunk: bytes):
        pass

    async def close(self):
        pass
