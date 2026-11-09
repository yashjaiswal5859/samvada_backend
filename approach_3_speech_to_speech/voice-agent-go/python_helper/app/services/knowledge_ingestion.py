import os
from pathlib import Path
import PyPDF2
from app.services.rag.rag_manager import rag_manager

class KnowledgeIngestionService:
    def __init__(self):
        self.docs_base_path = Path("D:/Voice AI/docs")

    async def ingest_company_docs(self, company_name: str, tenant_id: str, namespace: str = None) -> int:
        """
        Ingests documents into a FULLY ISOLATED RAG index.
        """
        company_path = self.docs_base_path / company_name
        if not company_path.exists():
            raise FileNotFoundError(f"Directory {company_path} does not exist.")

        # Get the isolated index for the namespace
        if namespace is None:
            namespace = company_name
        rag_index = rag_manager.get_index(namespace)

        docs_text = []
        for file_path in company_path.iterdir():
            if file_path.is_file():
                # Skip lead lists — they are NOT knowledge documents
                if file_path.suffix == ".csv" or file_path.suffix == ".xlsx":
                    continue
                if file_path.suffix == ".txt":
                    with open(file_path, "r", encoding="utf-8") as f:
                        docs_text.append({"text": f.read(), "source": file_path.name})
                elif file_path.suffix == ".pdf":
                    text = self._extract_pdf_text(file_path)
                    if text.strip():
                        docs_text.append({"text": text, "source": file_path.name})

        if docs_text:
            await rag_index.index_documents(docs_text, tenant_id)
            print(f"[KnowledgeIngestion] Ingested {len(docs_text)} docs into isolated namespace '{namespace}'.")
        return len(docs_text)

    def _extract_pdf_text(self, file_path: Path) -> str:
        text = ""
        try:
            with open(file_path, "rb") as f:
                reader = PyPDF2.PdfReader(f)
                for page in reader.pages:
                    text += page.extract_text() + "\n"
        except Exception as e:
            print(f"Failed to read PDF {file_path.name}: {e}")
        return text
