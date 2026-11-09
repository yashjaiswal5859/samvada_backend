from fastapi import APIRouter, Request, Response
from pydantic import BaseModel
import urllib.parse
from app.config import settings

router = APIRouter(prefix="/twiml", tags=["websocket", "twilio"])

@router.post("/inbound")
async def twilio_inbound(request: Request):
    # This endpoint receives the initial webhook from Twilio when a call comes in
    # It returns TwiML to connect the call to our WebSocket stream
    form_data = await request.form()
    call_sid = form_data.get("CallSid", "unknown_sid")
    customer_phone = form_data.get("From", "unknown_phone")
    
    # In a real app, you might want to identify the tenant ID from the 'To' number
    # For now, we'll pass a dummy tenant_id, or extract it from headers if possible
    tenant_id = "00000000-0000-0000-0000-000000000000"
    
    host = request.headers.get("host", "localhost:8000")
    ws_url = f"wss://{host}{settings.API_V1_PREFIX}/ws/stream/{call_sid}?tenant_id={tenant_id}&phone={urllib.parse.quote(customer_phone)}"
    
    twiml = f"""<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Connect>
        <Stream url="{ws_url}" />
    </Connect>
</Response>"""
    return Response(content=twiml, media_type="application/xml")
