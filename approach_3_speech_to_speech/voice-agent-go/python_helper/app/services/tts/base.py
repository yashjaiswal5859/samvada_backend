from abc import ABC, abstractmethod
from typing import AsyncGenerator

class BaseTTS(ABC):
    @abstractmethod
    async def stream_audio(self, text: str, voice: str) -> AsyncGenerator[bytes, None]:
        """
        Takes text and yields audio bytes to stream back to the caller.
        """
        pass
