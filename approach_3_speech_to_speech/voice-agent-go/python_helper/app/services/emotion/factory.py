from app.services.emotion.base import BaseAudioEmotionClassifier, BaseTextEmotionClassifier
from app.services.emotion.local_audio_emotion import LocalAudioEmotionClassifier
from app.services.emotion.local_text_emotion import LocalTextEmotionClassifier
from app.services.emotion.cloud_audio_emotion import CloudAudioEmotionClassifier
from app.services.emotion.cloud_text_emotion import CloudTextEmotionClassifier

class AudioEmotionFactory:
    @staticmethod
    def get_classifier(provider: str) -> BaseAudioEmotionClassifier:
        provider_lower = provider.lower() if provider else ""
        if "wav2vec2" in provider_lower or "emotion2vec" in provider_lower or "onnx" in provider_lower:
            return LocalAudioEmotionClassifier(model_name=provider)
        elif "hume" in provider_lower:
            return CloudAudioEmotionClassifier(provider=provider)
        else:
            raise ValueError(f"Unsupported Audio Emotion provider: {provider}")

class TextEmotionFactory:
    @staticmethod
    def get_classifier(provider: str) -> BaseTextEmotionClassifier:
        provider_lower = provider.lower() if provider else ""
        if "distilroberta" in provider_lower or "onnx" in provider_lower:
            return LocalTextEmotionClassifier(model_name=provider)
        elif "openai" in provider_lower or "gpt" in provider_lower:
            return CloudTextEmotionClassifier(model_name=provider)
        else:
            raise ValueError(f"Unsupported Text Emotion provider: {provider}")
