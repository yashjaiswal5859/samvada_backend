import threading
from app.services.rag.base import BaseRAG
from app.services.rag.local_rag import LocalRAG

class RAGManager:
    """
    Thread-safe singleton that maintains isolated FAISS indices per namespace.
    Namespace format: "{company_name}" for company-wide knowledge,
    or "{company_name}:{customer_phone}" for per-customer context.
    
    This guarantees ZERO data leakage between tenants/sessions.
    Each company gets its own completely separate vector index.
    """
    _instance = None
    _lock = threading.Lock()
    
    def __new__(cls):
        with cls._lock:
            if cls._instance is None:
                cls._instance = super().__new__(cls)
                cls._instance._indices = {}  # namespace -> LocalRAG instance
                cls._instance._index_lock = threading.Lock()
        return cls._instance
    
    def get_index(self, namespace: str) -> BaseRAG:
        """
        Returns an isolated RAG index for the given namespace.
        Creates one if it doesn't exist yet.
        """
        with self._index_lock:
            if namespace not in self._indices:
                self._indices[namespace] = LocalRAG()
                print(f"[RAGManager] Created isolated index for namespace: {namespace}")
            return self._indices[namespace]
    
    def drop_index(self, namespace: str):
        """Remove an index (e.g., after a campaign ends)."""
        with self._index_lock:
            if namespace in self._indices:
                del self._indices[namespace]
                print(f"[RAGManager] Dropped index for namespace: {namespace}")
    
    def list_namespaces(self) -> list[str]:
        """List all active namespaces."""
        with self._index_lock:
            return list(self._indices.keys())

# Global singleton
rag_manager = RAGManager()
