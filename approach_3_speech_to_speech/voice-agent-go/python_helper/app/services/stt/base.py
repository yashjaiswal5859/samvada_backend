from abc import ABC, abstractmethod
from typing import Callable

class BaseSTT(ABC):
    def __init__(self, callback: Callable):
        self.callback = callback

    @abstractmethod
    async def connect(self):
        """Establish connection to the STT provider."""
        pass

    @abstractmethod
    async def send_audio(self, audio_chunk: bytes):
        """Send audio chunk to the STT provider."""
        pass

    @abstractmethod
    async def close(self):
        """Close connection."""
        pass
