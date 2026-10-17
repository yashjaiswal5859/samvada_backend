# 🚀 Samvada Voice AI — Approach 3 Refactoring & HLD Alignment Roadmap

> **Document Status:** ACTIVE WORKING SPECIFICATION & IMPLEMENTATION TODO  
> **Source Architecture Reference:** [`d:/Voice AI/VOICE_AGENT_ARCHITECTURE_HLD_2026.md`](file:///d:/Voice%20AI/VOICE_AGENT_ARCHITECTURE_HLD_2026.md) (v1.1 FROZEN)  
> **Target Module:** `backend/approach_3_speech_to_speech/voice-agent-go`  
> **Created:** October 2026  
> **Author:** Antigravity AI Engineering Team

---

## 📌 Executive Summary

Approach 3 (`voice-agent-go`) contains robust low-latency foundations (pre-synthesized Turn 1 greeting audio, 20ms G.711 µ-law RTP pacer, acoustic background ambience mixing, and local fillers). 

However, a rigorous architectural comparison against **[VOICE_AGENT_ARCHITECTURE_HLD_2026.md](file:///d:/Voice%20AI/VOICE_AGENT_ARCHITECTURE_HLD_2026.md)** reveals several critical latency bottlenecks, race conditions on interruptions, unoptimized prompt caching, and missing state-machine guardrails.

This document serves as the **authoritative, actionable TODO checklist** to upgrade Approach 3 into full compliance with the 2026 HLD specification.

---

## 🔍 Gap Analysis: Current Code vs. HLD 2026 Specification

| Architectural Area | Current Approach 3 Implementation | HLD 2026 Authoritative Standard | Status |
|---|---|---|:---:|
| **Turn Detection Pacing** | Uses hardcoded **1,000ms debounce timer** (`time.AfterFunc(1000*time.Millisecond)`) and **1.5s artificial silence gap** on merged queries. | Turn Manager cross-references Deepgram `speech_final` with Silero VAD silence ($\ge 220\text{ms}$) with zero artificial delay. | 🔴 **CRITICAL** |
| **Barge-In Audio Race** | `pacer.ClearBuffer()` only flushes current queue. Delayed streaming LLM/TTS workers continue pushing old chunks with no generation filtering. | Monotonic `generation_seq uint64` per call. Media pacer discards any chunk where `frame.GenSeq < session.CurrentGenSeq`. | 🔴 **CRITICAL** |
| **Prompt Caching Alignment** | Ad-hoc prompt concatenation in `call_handler.go`. Floating `"model": "gpt-4o-mini"`. Dynamic customer fields mixed into system prompt prefix. | Strict **5-Tier modular structure**. Deterministic immutable prefix (Tiers 1 & 2 > 1,024 tokens) locked at head to ensure 100% cache hits. | 🔴 **CRITICAL** |
| **OpenAI Usage Telemetry** | `openAIChatResponse` only decodes `choices`. Completely ignores `usage.prompt_tokens_details.cached_tokens`. | Defensively parse `usage` and track `cached_tokens`, `uncached_tokens`, cache hit %, and stage timers (TTFT, STT tail, first TTS chunk). | 🟡 **HIGH** |
| **Model Lifecycle Governance** | Unpinned `gpt-4o-mini` with no deprecation cutover timeline. | Hard migration deadline: **November 30, 2026** (45 days before OpenAI Jan 14, 2027 snapshot retirement) with pre-cutover evaluation checklist. | 🟡 **HIGH** |
| **Conversation State Machine** | Raw chat history slice (`[]map[string]string`). Relies on LLM imagination for slot values and negation tracking. | Authoritative structured Go state machine (`SlotTracker`). Explicit proposed/rejected/confirmed slots; negations explicitly tracked. | 🟡 **HIGH** |
| **Fast-Path Intent Routing** | Ad-hoc regex for questions and greetings. Lacks dedicated policy-gated fast-path execution. | Dedicated Fast-Path Router (deterministic regex + calibrated Jev classifier) with authorization checks before action dispatch. | 🟡 **HIGH** |
| **Mutating Action Idempotency** | External CRM/ticket API calls do not pass idempotency keys. | Operation-scoped keys: $\text{SHA256}(\text{call\_id} + \text{turn\_seq} + \text{tool\_name} + \text{canonical\_args})$. | 🟢 **MEDIUM** |
| **Dual-Tier Transcripts** | Transcripts stored unredacted in local logs and Kafka. | Tier 1 (Operational PII-masked) for dashboards vs. Tier 2 (AES-256 encrypted Compliance Vault with TTL). | 🟢 **MEDIUM** |
| **Audio File Storage** | Saved as raw WAV files to local Windows disk (`D:/Voice AI/recordings`). | Finished session audio uploaded to S3-compatible object store (MinIO/AWS S3) with pre-signed access URLs. | 🟢 **MEDIUM** |

---

## 🛠️ Step-by-Step Implementation Roadmap

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ SPRINT 1: CORE ENGINE HARDENING & LATENCY PURGE                             │
│ ├─ Task 1.1: Purge 1,000ms Debounce & Implement Fast Turn Taking            │
│ ├─ Task 1.2: Add Monotonic Generation Sequence IDs (generation_seq)         │
│ └─ Task 1.3: Instrument Server-Side TAT & Stage Timers                      │
├─────────────────────────────────────────────────────────────────────────────┤
│ SPRINT 2: PROMPT MODULARIZATION & CACHING TELEMETRY                         │
│ ├─ Task 2.1: Refactor Prompt Construction into 5 Strict Tiers               │
│ ├─ Task 2.2: Parse OpenAI Cached-Tokens Usage & Log Cache Hit Metrics       │
│ └─ Task 2.3: Pin Model Snapshot & Enforce Deprecation Cutover Governance    │
├─────────────────────────────────────────────────────────────────────────────┤
│ SPRINT 3: AUTHORITATIVE STATE MACHINE & FAST-PATH ROUTING                   │
│ ├─ Task 3.1: Build Go Conversation State Machine (Slot-Tracker & Negations) │
│ └─ Task 3.2: Implement Fast-Path Router Module with Policy Gates            │
├─────────────────────────────────────────────────────────────────────────────┤
│ SPRINT 4: IDEMPOTENCY, SECURITY & CLOUD STORAGE                             │
│ ├─ Task 4.1: Inject Operation-Scoped Idempotency Keys on External Tools     │
│ ├─ Task 4.2: Implement Dual-Tier Transcripts (PII-Masking & AES-256 Vault)   │
│ └─ Task 4.3: Migrate Local Audio Storage to MinIO/S3 with Pre-signed URLs   │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 📝 Detailed Action Items by File

### 1. `internal/services/media_pacer.go`
- [ ] **Add Monotonic Sequence Counter:**
  ```go
  type MediaPacer struct {
      ...
      currentGenSeq uint64 // Monotonically increments on every turn / barge-in
  }
  ```
- [ ] **Add Sequence Increment & Audio Purge Method:**
  ```go
  func (p *MediaPacer) IncrementGenSeq() uint64 {
      p.mu.Lock()
      defer p.mu.Unlock()
      p.currentGenSeq++
      p.audioBuffer = nil // Instantly drop all queued frames
      // Send Twilio/Plivo clearAudio WebSocket frame
      ...
      return p.currentGenSeq
  }
  ```
- [ ] **Add Sequence-Aware Audio Enqueueing:**
  ```go
  func (p *MediaPacer) PushAudioWithGen(data []byte, genSeq uint64) {
      p.mu.Lock()
      defer p.mu.Unlock()
      if genSeq < p.currentGenSeq {
          // Discard stale frame immediately before RTP packetization
          return
      }
      p.audioBuffer = append(p.audioBuffer, data...)
  }
  ```
- [ ] **Maintain Backward-Compatible Helper:**
  `func (p *MediaPacer) PushAudio(data []byte)` delegates to `PushAudioWithGen(data, p.CurrentGenSeq())`.

---

### 2. `internal/api/websocket/call_handler.go`
- [ ] **Purge Artificial Latency Buffers:**
  - Delete line 707 (`debounceTimer = time.AfterFunc(1000*time.Millisecond)`).
  - Delete line 1014 (`silenceGap := bytes.Repeat([]byte{0xFF}, 12000)`).
  - Reduce `pauseWindow` from 650ms to the authoritative 220ms acoustic silence threshold.
- [ ] **Deepgram `speech_final` vs `is_final` Turn Taking:**
  - In `sttSession.Transcripts()`:
    - If `res.IsFinal`, append text chunk to `accumulatedTranscript`.
    - If `res.SpeechFinal`, check acoustic silence duration from Silero VAD ($\ge 220\text{ms}$).
    - If confirmed silence $\ge 220\text{ms}$ with no prosodic continuation, fire `triggerTurn()`.
    - If caller resumes speaking during a pause, abort pending trigger and keep accumulating.
- [ ] **Instant Barge-In Handling:**
  - When caller speech activity is verified (`services.IsSpeechFrame(payloadBytes)` with `speechFrameCount >= 3`) while `pacer.IsPlaying()`:
    - Call `activeGen := pacer.IncrementGenSeq()`.
    - Trigger `activeTurnCancel()` to abort in-flight LLM HTTP stream and Cartesia TTS worker.
    - Set `isSpeaking = true` and accumulate caller audio immediately (do not drop user audio).
- [ ] **Server-Side Turn-Around Time (TAT) Instrumentation:**
  - Track `t_last_voiced`: timestamp of caller's last voiced audio frame.
  - Track `t_first_outbound`: timestamp of first response frame handed to `pacer`.
  - Calculate `ServerTAT = t_first_outbound.Sub(t_last_voiced)`.
  - Log and emit histogram metrics partitioned by execution profile:
    - Profile A (Fast-Path): target ~260–320ms.
    - Profile B (Normal Generative): target ~350–480ms.
    - Profile C (External Tools): target ~865–1,315ms.

---

### 3. `internal/services/prompt_service.go` & LLM Client
- [ ] **Refactor Prompt Construction into 5 Logical Tiers:**
  - **Tier 1 (Immutable Persona):** Stable identity, guidelines, pronunciation rules.
  - **Tier 2 (Immutable Tools):** Tool definitions, argument schemas.
  - **Tier 3 (Call-Scoped):** Customer identity, product, campaign goal (set at call start).
  - **Tier 4 (Turn-Scoped RAG):** Dynamic vector search results (injected *only* when question requires knowledge).
  - **Tier 5 (Dynamic State & History):** Validated structured state + last 2 dialogue turns + latest utterance.
- [ ] **Model Lifecycle Governance:**
  - Pin primary model to `gpt-4o-mini-2024-07-18` (or active pointer).
  - Set migration governance cutover milestone: **November 30, 2026**.
  - Document pre-cutover checklist: quality, tool schema compliance, TTFT, prompt cache hit rate, and cost.
- [ ] **Parse OpenAI Prompt Caching Usage Details:**
  - Update `openAIChatResponse`:
    ```go
    type openAIChatResponse struct {
        Choices []openAIChoice `json:"choices"`
        Usage   struct {
            PromptTokens        int `json:"prompt_tokens"`
            CompletionTokens    int `json:"completion_tokens"`
            TotalTokens         int `json:"total_tokens"`
            PromptTokensDetails struct {
                CachedTokens int `json:"cached_tokens"`
            } `json:"prompt_tokens_details"`
        } `json:"usage"`
    }
    ```
  - Log `prompt_tokens`, `cached_tokens`, and calculate cache hit percentage:
    $$\text{Cache Hit \%} = \frac{\text{CachedTokens}}{\text{PromptTokens}} \times 100$$

---

### 4. `internal/models/conversation_state.go` & State Machine
- [ ] **Create Authoritative State Machine Struct:**
  ```go
  type ConversationState struct {
      Intent              string            `json:"intent"`
      ConfirmedSlots      map[string]string `json:"confirmed_slots"`
      RejectedSlots       map[string]string `json:"rejected_slots"`
      PendingConfirmation string            `json:"pending_confirmation"`
      BookingStatus       string            `json:"booking_status"`
      TurnSequence        uint64            `json:"turn_sequence"`
  }
  ```
- [ ] **Negation Protection Logic:**
  - If user explicitly rejects a slot (*"Not Tuesday"*, *"Don't call after 5 PM"*), store in `RejectedSlots`.
  - Validate LLM extraction before committing: reject any LLM transition that attempts to confirm a rejected slot.

---

### 5. `internal/services/fast_path_router.go`
- [ ] **Implement Deterministic Fast-Path Interceptor:**
  - Evaluate incoming utterance against deterministic policies:
    - `"transfer to agent"`, `"speak with human"`, `"representative"` $\rightarrow$ Human queue dispatch.
    - `"stop calling me"`, `"don't call again"`, `"opt out"` $\rightarrow$ DNC register update & polite hangup.
    - Simple affirmations (*"yes"*, *"sure"*, *"confirm"*) for pending confirmation slots.
- [ ] **Authorization Discipline Gate:**
  - Check caller authorization and policy rules before dispatching.
  - If intent is ambiguous or low confidence, immediately fall back to the full generative LLM pipeline.

---

### 6. `internal/services/customer_service.go` & `ticket_service.go`
- [ ] **Implement Operation-Scoped Idempotency Keys:**
  ```go
  func GenerateIdempotencyKey(callID string, turnSeq uint64, toolName string, argsHash string) string {
      raw := fmt.Sprintf("%s:%d:%s:%s", callID, turnSeq, toolName, argsHash)
      hash := sha256.Sum256([]byte(raw))
      return hex.EncodeToString(hash[:])
  }
  ```
- [ ] Pass `Idempotency-Key: <key>` header on all external CRM, ticketing, and calendar API requests.

---

### 7. `internal/services/audio_recorder.go` & Transcript Store
- [ ] **Dual-Tier Transcript Engine:**
  - **Operational Tier:** Apply regex and NER masking to redact credit card numbers, tax IDs, and sensitive PII before writing to standard PostgreSQL / ClickHouse.
  - **Compliance Vault Tier:** Store unredacted transcripts encrypted with AES-256 in a dedicated vault with automated TTL deletion.
- [ ] **S3-Compatible Audio Archival:**
  - Replace raw Windows disk write with an async S3/MinIO uploader.
  - Generate 1-hour pre-signed URLs for dashboard audio playback.

---

## 📊 Verification & Acceptance Criteria

When all action items are implemented, the system must meet the following empirical acceptance criteria:

1. **TAT Latency Compliance:**
   - **Profile A (Fast-Path):** P50 $\le 280\text{ms}$, P95 $\le 320\text{ms}$.
   - **Profile B (Generative):** P50 $\le 420\text{ms}$, P95 $\le 480\text{ms}$.
   - **Profile C (External Tools):** P50 $\le 950\text{ms}$, P95 $\le 1,300\text{ms}$.
2. **Zero Audio Bleed on Interruption:**
   - Interrupting mid-speech purges audio buffer in $< 20\text{ms}$, and no delayed chunks from the cancelled turn play over subsequent turns.
3. **Prompt Cache Verification:**
   - Measured `usage.prompt_tokens_details.cached_tokens` verifies $\ge 60\%\text{--}80\%$ cache hit rates across turns 2+ of active calls.
4. **State Machine Integrity:**
   - Negations (*"Not Tuesday"*) never commit as confirmed slots.
5. **Idempotency Guarantee:**
   - Retried tool calls produce zero duplicate database records or tickets.

---

> **Next Action:** Begin execution with **Sprint 1 (Priority 1: Latency Purge & Turn Taking + Priority 2: Monotonic `generation_seq` in `media_pacer.go`)**.
