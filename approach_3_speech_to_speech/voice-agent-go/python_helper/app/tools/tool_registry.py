from typing import Callable, Any
from app.schemas.tool import ToolCallRequest, ToolCallResult
import time

class ToolRegistry:
    def __init__(self):
        self._tools = {}

    def register(self, name: str, func: Callable):
        self._tools[name] = func

    async def execute(self, request: ToolCallRequest) -> ToolCallResult:
        start_time = time.time()
        func = self._tools.get(request.tool_name)
        
        if not func:
            return ToolCallResult(
                tool_name=request.tool_name,
                status="failure",
                result={"error": f"Tool '{request.tool_name}' not found"},
                duration_ms=int((time.time() - start_time) * 1000)
            )
            
        try:
            # Assuming tools are async
            res = await func(**request.parameters)
            status = "success"
        except Exception as e:
            res = {"error": str(e)}
            status = "failure"
            
        duration = int((time.time() - start_time) * 1000)
        return ToolCallResult(
            tool_name=request.tool_name,
            status=status,
            result=res,
            duration_ms=duration
        )

# Global registry instance
tool_registry = ToolRegistry()
