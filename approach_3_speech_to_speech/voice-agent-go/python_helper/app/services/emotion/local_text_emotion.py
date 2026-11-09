from app.services.emotion.base import BaseTextEmotionClassifier

class LocalTextEmotionClassifier(BaseTextEmotionClassifier):
    def __init__(self, model_name: str = "distilroberta"):
        self.model_name = model_name
        # Initialize local transformers pipeline or ONNX session for distilroberta-base-emotion
        print(f"[LocalTextEmotion] Loaded local model: {self.model_name}")

    async def classify_text(self, transcript_segment: str) -> dict[str, float]:
        # Placeholder for model inference
        # DistilRoBERTa would parse the text and output a probability distribution
        return {
            "anger": 0.05,
            "joy": 0.15,
            "sadness": 0.05,
            "neutral": 0.75
        }
