from abc import ABC, abstractmethod

class BaseLLM(ABC):
    @abstractmethod
    async def generate_response(self, messages: list[dict], model: str) -> str:
        """
        Takes a list of message dicts and a model string, and returns the response string.
        """
        pass
