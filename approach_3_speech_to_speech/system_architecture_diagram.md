# System Architecture & End-to-End Data Flow: Voice AI Agent

```mermaid
flowchart TB
    subgraph PSTN ["1. PSTN Telephony Layer"]
        User(["📱 Caller Phone"])
        Plivo["Plivo / Twilio Provider"]
        Tunnel["Cloudflare Tunnel (cloudflared)"]
    end

    subgraph GoServer ["2. Go Core Application Engine (voice-agent-go)"]
        direction TB
        
        subgraph Handlers ["Connection & Session Handlers"]
            APIHandler["Call API Handler<br/>(Answer-URL XML: <Stream>)"]
            WSHandler["Call WebSocket Handler<br/>(Full-Duplex Stream)"]
        end

        subgraph AudioProcess ["Audio Processing & Pacing Engine"]
            DRC["Dynamic Gain / DRC<br/>(ApplyDynamicGain +11dB)"]
            RingBuf["300ms Pre-Speech Ring Buffer<br/>(Zero Syllable Clipping)"]
            Pacer["Media Pacer (20ms PSTN Clock)<br/>+ Ambience Audio Mixer"]
        end

        subgraph Pipeline ["Speech & NLP Pipeline"]
            Norm["Query Normalizer<br/>(Devanagari + Soundex + Levenshtein)"]
            Debounce["Utterance Debouncer<br/>(1000ms Silence Gap Timer)"]
        end
    end

    subgraph ExternalServices ["3. Cloud & Local AI Services (Factory Pattern)"]
        Deepgram["🎙️ Deepgram STT (Nova-2 WS)<br/>Sub-150ms + Keyword Boost"]
        FishTTS["🔊 Fish Audio TTS<br/>Sub-180ms Streaming 8kHz mu-law"]
        OpenAILLM["🧠 OpenAI GPT-4o-mini<br/>Streaming Token Generator"]
    end

    subgraph RAM_Storage ["4. In-Memory RAG & Session State"]
        LocalONNX["⚡ Local C++ ONNX RAG<br/>all-MiniLM-L6-v2 (384dim, <5ms)"]
        RedisCache["🔴 Redis Cache<br/>0ms Opening Audio & Call Config"]
        DocVocab["📚 Doc Vocabulary Index<br/>Domain Term & Phonetic Map"]
    end

    %% Connections
    User <-->|PSTN Voice Call| Plivo
    Plivo <-->|HTTP Answer-URL XML / WS Stream| Tunnel
    Tunnel <-->|Full-Duplex mu-law Stream| WSHandler
    APIHandler -->|Read Config & Pre-Synth Greetings| RedisCache

    WSHandler --> DRC
    DRC --> RingBuf
    RingBuf --> Deepgram
    
    Deepgram -->|IsFinal Transcript| Debounce
    Debounce --> Norm
    Norm <-->|Domain Dictionary Lookup| DocVocab

    Norm -->|Normalized Query Sentence| LocalONNX
    LocalONNX -->|65% Vector + 35% BM25 Context| OpenAILLM

    OpenAILLM -->|Stream Clauses| FishTTS
    FishTTS -->|8kHz Audio Chunks| Pacer
    Pacer <-->|Mix Office Ambience Loop| WSHandler

    style PSTN fill:#1e293b,color:#fff,stroke:#3b82f6
    style GoServer fill:#0f172a,color:#fff,stroke:#10b981
    style ExternalServices fill:#1e1b4b,color:#fff,stroke:#6366f1
    style RAM_Storage fill:#1c1917,color:#fff,stroke:#f59e0b
```

---

## 30-Second Architecture Walkthrough Script (When Screen Sharing)

> *"Here is the complete end-to-end system architecture:*
>
> 1. **Telephony & Media Layer**: When a call comes in over **Plivo**, it hits our **Cloudflare Tunnel** and connects to our Go **`CallWebSocketHandler`** for full-duplex 8kHz audio streaming.
> 2. **Audio Processing**: Incoming audio passes through **Dynamic Range Compression** (to amplify quiet callers) and a **300ms Rolling Ring Buffer** (to prevent initial syllable clipping) before hitting **Deepgram STT** via WebSocket.
> 3. **Normalizer & Hybrid RAG**: When Deepgram emits a final transcript, our **`QueryNormalizer`** fixes Hinglish typos and phonetic errors (*'aco'* $\rightarrow$ *'SEO'*) in <0.5ms using our domain vocabulary index. The query is then sent to our **Local C++ ONNX Hybrid RAG** running in server RAM (<5ms search).
> 4. **LLM & Streaming TTS**: Retrieved context is injected into **GPT-4o-mini**, which streams tokens directly to **Fish Audio TTS**.
> 5. **Pacing & Ambience**: **`MediaPacer`** paces audio chunks at 20ms PSTN frame intervals while continuously mixing background office ambience to ensure the phone line never sounds dead."*
