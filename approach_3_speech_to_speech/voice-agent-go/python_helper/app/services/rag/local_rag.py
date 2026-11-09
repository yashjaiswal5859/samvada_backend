from app.services.rag.base import BaseRAG
from app.schemas.rag import RAGQuery, RAGResult
import faiss
import numpy as np
import uuid

class LocalRAG(BaseRAG):
    def __init__(self):
        # Initialize a basic in-memory FAISS index
        # 1536 is standard OpenAI embedding dimension
        self.dimension = 1536
        self.index = faiss.IndexFlatL2(self.dimension)
        
        # Store metadata mapping (index_id -> metadata_dict)
        self.metadata_store = {}
        self.current_idx = 0

    async def _get_embedding(self, text: str) -> np.ndarray:
        # Placeholder for OpenAI text-embedding-3-small call
        # Return a random numpy array for structural testing
        return np.random.rand(self.dimension).astype('float32')

    async def index_documents(self, docs: list[dict], tenant_id: str):
        for doc in docs:
            # Chunking logic would go here (e.g. splitting by 500 words)
            text_chunk = doc["text"][:1000] # Naive chunking
            embedding = await self._get_embedding(text_chunk)
            
            # Add to FAISS
            self.index.add(np.array([embedding]))
            
            # Store metadata
            self.metadata_store[self.current_idx] = {
                "tenant_id": tenant_id,
                "text": text_chunk,
                "source": doc["source"]
            }
            self.current_idx += 1

    async def search(self, query: RAGQuery) -> list[RAGResult]:
        if self.current_idx == 0:
            return []
            
        q_embedding = await self._get_embedding(query.query)
        
        # Search FAISS
        distances, indices = self.index.search(np.array([q_embedding]), k=3)
        
        results = []
        for dist, idx in zip(distances[0], indices[0]):
            if idx != -1:
                meta = self.metadata_store[idx]
                # Filter by tenant_id
                if meta["tenant_id"] == str(query.tenant_id):
                    # Convert L2 distance to a faux similarity score (0 to 1)
                    score = 1.0 / (1.0 + float(dist))
                    results.append(
                        RAGResult(
                            text=meta["text"],
                            score=score,
                            source=meta["source"],
                            metadata={"tenant_id": meta["tenant_id"]}
                        )
                    )
        return results
