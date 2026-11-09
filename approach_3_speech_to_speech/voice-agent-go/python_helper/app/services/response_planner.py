from app.schemas.strategy import StrategyOutput
from app.schemas.tts import ResponsePlannerOutput, TTSStyle

class ResponsePlanner:
    def __init__(self):
        pass
        
    def determine_tts_style(self, llm_text: str, strategy: StrategyOutput) -> ResponsePlannerOutput:
        """
        Deterministically maps the StrategyOutput to specific TTS vocal characteristics.
        This completely bypasses the LLM for voice stylization, ensuring low latency.
        """
        # Default baseline
        style = TTSStyle(
            tone="professional",
            pitch="medium",
            rate="medium",
            energy="medium",
            pause="normal"
        )
        
        if strategy:
            conv_strat = strategy.conversation_strategy
            cust_state = strategy.customer_state
            
            # Map Pace
            if conv_strat.pace == "fast":
                style.rate = "1.2x"
            elif conv_strat.pace == "slow" or cust_state.stress > 0.7:
                style.rate = "0.8x"
                style.pause = "extended" # More pauses for stressed customers
                
            # Map Energy
            if conv_strat.energy == "high":
                style.energy = "enthusiastic"
            elif conv_strat.energy == "low" or cust_state.stress > 0.7:
                style.energy = "calm"
                style.pitch = "lower"
                
            # Tone overrides
            if cust_state.emotion == "frustrated":
                style.tone = "empathetic"
                style.rate = "0.9x"
                
        return ResponsePlannerOutput(
            text=llm_text,
            tts_style=style
        )
