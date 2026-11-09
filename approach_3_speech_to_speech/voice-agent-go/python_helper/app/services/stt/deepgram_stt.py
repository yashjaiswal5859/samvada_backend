from app.services.stt.base import BaseSTT
from deepgram import DeepgramClient, LiveOptions
from app.config import settings

class DeepgramSTT(BaseSTT):
    def __init__(self, callback):
        super().__init__(callback)
        self.dg_client = DeepgramClient(settings.DEEPGRAM_API_KEY)
        self.connection = None

    async def connect(self):
        try:
            self.connection = self.dg_client.listen.asyncwebsocket.v("1")
            
            self.connection.on("Results", self.on_message)
            self.connection.on("Error", self.on_error)
            
            options = LiveOptions(
                model="nova-2",
                language="multi",
                smart_format=True,
                encoding="mulaw",
                sample_rate=8000,
                channels=1,
                endpointing=10
            )
            await self.connection.start(options)
        except Exception as e:
            print(f"STT Connect error: {e}")

    async def on_message(self, result, **kwargs):
        sentence = result.channel.alternatives[0].transcript
        is_final = result.is_final
        if sentence:
            await self.callback(sentence, is_final)

    async def on_error(self, error, **kwargs):
        print(f"STT Error: {error}")

    async def send_audio(self, audio_chunk: bytes):
        if self.connection:
            await self.connection.send(audio_chunk)

    async def close(self):
        if self.connection:
            await self.connection.finish()
