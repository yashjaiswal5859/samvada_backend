from app.services.llm.factory import LLMFactory
from app.services.prompt_builder import PromptBuilder
from app.schemas.conversation import ConversationState
from app.schemas.strategy import StrategyOutput
from app.config import settings

class LLMService:
    def __init__(self):
        self.prompt_builder = PromptBuilder()
        
    async def generate_turn(self, state: ConversationState, strategy: StrategyOutput, latest_user_input: str) -> str:
        """
        Generates the actual conversational response text.
        It uses the prompt builder to strictly enforce constraints.
        """
        messages = self.prompt_builder.build_prompt(state, strategy, latest_user_input)
        
        # Determine model from settings or logic
        target_model = settings.DEFAULT_LLM_MODEL
        
        # Get concrete LLM provider instance from factory
        llm_provider = LLMFactory.get_llm(target_model)
        
        # Generate
        response_text = await llm_provider.generate_response(messages, target_model)
        
        return response_text
