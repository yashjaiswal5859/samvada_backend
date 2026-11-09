from app.services.rag.base import BaseRAG
from app.services.rag.pinecone_rag import PineconeRAG
from app.services.rag.local_rag import LocalRAG

class RAGFactory:
    @staticmethod
    def get_rag(provider: str = "local") -> BaseRAG:
        provider_lower = provider.lower() if provider else "local"
        if "pinecone" in provider_lower:
            return PineconeRAG()
        elif "local" in provider_lower or "all-minilm" in provider_lower or "onnx" in provider_lower:
            return LocalRAG()
        else:
            raise ValueError(f"Unsupported RAG provider: {provider}")
