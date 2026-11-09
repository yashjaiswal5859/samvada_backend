from app.schemas.conversation import ConversationState
from app.schemas.strategy import StrategyOutput
from typing import Optional

class PromptBuilder:
    def __init__(self):
        # Base system prompt that NEVER changes emotion
        self.base_system_prompt = (
            "You are an AI Sales Agent named Anjali representing Swiggy. "
            "Your sole objective is to pitch and sell the 'Swiggy HDFC Bank Credit Card' to yash by convincing him of its value. "
            "Do NOT go off-track. Keep the conversation focused entirely on selling this credit card. "
            "Your personality must be extremely positive, polite, enthusiastic, and highly persuasive. "
            "You must rely strictly and exclusively on the retrieved context from the vector database for all card details (such as cashback percentages, fees, waivers, limits, and eligibility). "
            "If yash asks a question that is not answered in the retrieved context, politely inform him that you do not have that specific detail on hand, and redirect the conversation back to the credit card benefits."
        )
        
    def build_prompt(self, state: ConversationState, strategy: Optional[StrategyOutput], latest_user_input: str) -> list[dict]:
        """
        Assembles the strict prompt for the LLM. 
        It injects the strategy constraints dynamically without telling the LLM to 'be sad' or 'be angry'.
        """
        messages = [{"role": "system", "content": self.base_system_prompt}]
        
        if strategy:
            strategy_instructions = (
                f"\n\nCURRENT CONVERSATION STRATEGY:\n"
                f"- Tone: Keep it {strategy.conversation_strategy.tone}\n"
                f"- Pace: {strategy.conversation_strategy.pace}\n"
                f"- Verbosity: {strategy.conversation_strategy.verbosity}\n"
                f"- Primary Goal: {strategy.conversation_strategy.goal}\n"
            )
            if strategy.conversation_strategy.active_listening:
                strategy_instructions += "- Instruction: Acknowledge the user's situation before answering.\n"
                
            messages[0]["content"] += strategy_instructions
            
        # Append limited history (e.g., last 5 turns) to save tokens
        recent_history = state.transcript[-10:] if len(state.transcript) > 10 else state.transcript
        for msg in recent_history:
            messages.append({"role": msg.role, "content": msg.content})
            
        # Append latest input
        messages.append({"role": "user", "content": latest_user_input})
        
        return messages
