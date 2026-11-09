import pytest
import uuid

@pytest.mark.asyncio
async def test_rag_isolation_and_persistence(async_client):
    tenant_id = uuid.uuid4()
    source_call_id = uuid.uuid4()
    
    session_yash = "yash_session_123"
    session_ram = "ram_session_456"
    
    # 1. Initialize sessions
    resp = await async_client.post("/v2/rag/session/initialize", json={
        "session_id": session_yash,
        "company_name": "swiggy",
        "tenant_id": str(tenant_id),
        "phone": "+11111"
    })
    assert resp.status_code == 200
    
    resp = await async_client.post("/v2/rag/session/initialize", json={
        "session_id": session_ram,
        "company_name": "swiggy",
        "tenant_id": str(tenant_id),
        "phone": "+22222"
    })
    assert resp.status_code == 200

    # 2. Index live dynamic preference for Yash
    resp = await async_client.post("/v2/rag/session/index", json={
        "session_id": session_yash,
        "text": "Yash prefers dine out option",
        "source": "live_transcript",
        "tenant_id": str(tenant_id)
    })
    assert resp.status_code == 200

    # 3. Index live dynamic preference for Ram
    resp = await async_client.post("/v2/rag/session/index", json={
        "session_id": session_ram,
        "text": "Ram prefers free delivery option",
        "source": "live_transcript",
        "tenant_id": str(tenant_id)
    })
    assert resp.status_code == 200

    # 4. Search Session Yash - Verify isolated result (should NOT find Ram's preference)
    resp = await async_client.post("/v2/rag/session/search", json={
        "session_id": session_yash,
        "query": "prefers dine out",
        "tenant_id": str(tenant_id)
    })
    assert resp.status_code == 200
    results = resp.json()
    assert len(results) > 0
    assert "Yash" in results[0]["text"]
    assert "Ram" not in results[0]["text"]

    # Search Session Ram - Verify isolated result (should NOT find Yash's preference)
    resp = await async_client.post("/v2/rag/session/search", json={
        "session_id": session_ram,
        "query": "prefers free delivery",
        "tenant_id": str(tenant_id)
    })
    assert resp.status_code == 200
    results = resp.json()
    assert len(results) > 0
    assert "Ram" in results[0]["text"]
    assert "Yash" not in results[0]["text"]

    # 5. Persist Yash's preference to Postgres
    resp = await async_client.post("/v2/rag/session/persist", json={
        "session_id": session_yash,
        "phone": "+11111",
        "tenant_id": str(tenant_id),
        "memories": ["Yash prefers dine out option"],
        "source_call_id": str(source_call_id)
    })
    assert resp.status_code == 200

    # 6. Destroy current in-memory sessions
    resp = await async_client.delete(f"/v2/rag/session/{session_yash}")
    assert resp.status_code == 200
    
    resp = await async_client.delete(f"/v2/rag/session/{session_ram}")
    assert resp.status_code == 200

    # 7. Initialize a NEW session for Yash (+11111) - should auto-load historical memory from Postgres
    session_yash_new = "yash_session_789"
    resp = await async_client.post("/v2/rag/session/initialize", json={
        "session_id": session_yash_new,
        "company_name": "swiggy",
        "tenant_id": str(tenant_id),
        "phone": "+11111"
    })
    assert resp.status_code == 200

    # 8. Search new session - verify historical memory was successfully loaded
    resp = await async_client.post("/v2/rag/session/search", json={
        "session_id": session_yash_new,
        "query": "dine out option",
        "tenant_id": str(tenant_id)
    })
    assert resp.status_code == 200
    results = resp.json()
    assert len(results) > 0
    assert "Yash prefers dine out option" in results[0]["text"]
    
    # 9. Cleanup
    resp = await async_client.delete(f"/v2/rag/session/{session_yash_new}")
    assert resp.status_code == 200
