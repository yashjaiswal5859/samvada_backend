from app.services.llm.base import BaseLLM
from app.services.llm.openai_llm import OpenAILLM
from app.services.llm.anthropic_llm import AnthropicLLM
from app.services.llm.gemini_llm import GeminiLLM

class LLMFactory:
    @staticmethod
    def get_llm(model: str) -> BaseLLM:
        model_lower = model.lower() if model else ""
        if "gpt" in model_lower or "openai" in model_lower:
            return OpenAILLM()
        elif "claude" in model_lower or "anthropic" in model_lower:
            return AnthropicLLM()
        elif "gemini" in model_lower or "google" in model_lower:
            return GeminiLLM()
        else:
            raise ValueError(f"Unsupported model prefix for: {model}")
