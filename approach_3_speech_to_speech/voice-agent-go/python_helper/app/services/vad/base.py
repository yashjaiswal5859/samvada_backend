from abc import ABC, abstractmethod

class BaseVAD(ABC):
    @abstractmethod
    def process_audio(self, audio_chunk: bytes) -> bool:
        """
        Process incoming audio chunk and return True if human speech is detected.
        Used for barge-in (interruption) detection.
        """
        pass
