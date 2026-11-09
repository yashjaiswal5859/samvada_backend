from app.tools.tool_registry import tool_registry

async def book_appointment(date: str, time: str, customer_name: str) -> dict:
    """
    Example tool to book an appointment.
    """
    # TODO: Implement idempotency using Redis
    # TODO: Make external API call to Cal.com or Calendly
    
    return {
        "status": "confirmed",
        "booking_id": "BK-12345",
        "message": f"Appointment booked for {customer_name} on {date} at {time}."
    }

tool_registry.register("book_appointment", book_appointment)
