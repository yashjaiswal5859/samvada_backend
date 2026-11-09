from abc import ABC, abstractmethod
from app.schemas.rag import RAGQuery, RAGResult

class BaseRAG(ABC):
    @abstractmethod
    async def search(self, query: RAGQuery) -> list[RAGResult]:
        """
        Embeds the query and searches the vector database.
        Returns top-k chunks for the LLM to use.
        """
        pass

    @abstractmethod
    async def index_documents(self, docs: list[dict], tenant_id: str):
        """
        Takes raw text documents and embeds/indexes them for a specific tenant.
        """
        pass
