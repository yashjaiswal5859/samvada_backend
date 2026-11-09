import re

# Basic regex for PII (for illustration)
PII_PATTERNS = {
    "PHONE_REDACTED": r"\b(?:\+?1[-.\s]?)?\(?[2-9]\d{2}\)?[-.\s]?\d{3}[-.\s]?\d{4}\b",
    "EMAIL_REDACTED": r"\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,7}\b",
    "SSN_REDACTED": r"\b\d{3}-\d{2}-\d{4}\b",
    "CREDIT_CARD_REDACTED": r"\b(?:\d[ -]*?){13,16}\b"
}

def redact(text: str) -> tuple[str, list[str]]:
    """
    Redacts PII from text and returns the redacted text along with a list of PII types detected.
    """
    redacted_text = text
    detected_types = []
    
    for pii_type, pattern in PII_PATTERNS.items():
        if re.search(pattern, redacted_text):
            detected_types.append(pii_type)
            redacted_text = re.sub(pattern, f"[{pii_type}]", redacted_text)
            
    return redacted_text, detected_types
