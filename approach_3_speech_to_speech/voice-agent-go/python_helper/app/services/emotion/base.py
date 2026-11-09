from abc import ABC, abstractmethod

class BaseAudioEmotionClassifier(ABC):
    @abstractmethod
    async def classify_audio(self, audio_chunk_16k: bytes) -> dict[str, float]:
        """
        Takes raw 16kHz PCM audio bytes (sliding window) and returns 
        a dictionary mapping emotion categories to probability scores (0.0 to 1.0).
        """
        pass

class BaseTextEmotionClassifier(ABC):
    @abstractmethod
    async def classify_text(self, transcript_segment: str) -> dict[str, float]:
        """
        Takes a running transcript string and returns
        a dictionary mapping emotion categories to probability scores (0.0 to 1.0).
        """
        pass
