from app.services.emotion.base import BaseTextEmotionClassifier
from app.services.llm.factory import LLMFactory
from app.config import settings

class CloudTextEmotionClassifier(BaseTextEmotionClassifier):
    def __init__(self, model_name: str = "gpt-4o-mini"):
        self.model_name = model_name
        self.llm = LLMFactory.get_llm(model_name)

    async def classify_text(self, transcript_segment: str) -> dict[str, float]:
        # Call the LLM to get structured emotion output for the intermediate transcript
        # This is a fallback/cloud approach if we don't run local models
        prompt = [
            {"role": "system", "content": "Analyze the sentiment of the user transcript. Output json format with keys: anger, joy, sadness, neutral scoring 0 to 1."},
            {"role": "user", "content": transcript_segment}
        ]
        # In a real implementation, we parse the JSON response from the LLM
        return {
            "anger": 0.0,
            "joy": 0.1,
            "sadness": 0.0,
            "neutral": 0.9
        }
