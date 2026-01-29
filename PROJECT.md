# Project Contract: Go Agent Orchestrator (Free Stack)

## 1) Problem Statement
Today’s LLM “agents” (LangChain-style) are often brittle: they crash, loop, hallucinate tool usage, and are difficult to replay or debug. This project builds a durable agent orchestration runtime in Go that persists agent state, logs tool calls, detects failures, and can automatically repair common failure modes — using only free, local, open-source components.

## 2) Target User
A developer who wants to run an LLM agent reliably on their own machine (no paid APIs), with replayable runs, observable behavior, and a path to reduce hallucinations through validation + experiments.

## 3) Key Guarantees (What the system must always do)
- Persist every agent run and step so it can be resumed and replayed.
- Record every tool call request and tool response.
- Enforce timeouts and retries for tool calls.
- Validate agent outputs and never silently accept invalid outputs.
- Provide a clear, structured final result for the target task.

## 4) Non-Goals (Not doing in v1)
- Not building a full LangChain replacement.
- Not building a fancy UI (CLI or simple HTTP is enough).
- Not doing multi-agent swarms in v1.
- Not relying on paid APIs or cloud services.

## 5) Free Technical Stack (Initial)
- Go for orchestration + APIs
- Local LLM via Ollama
- Local embeddings via Ollama embedding models
- SQLite for persistence (upgradeable to Postgres later)
- Simple local logging/metrics

## 6) Golden Path Use Case (One demo scenario)

Golden Path: Local Log Analysis Agent

Goal:
Given a directory of application log files on the local machine, the agent analyzes the logs, identifies the most likely root cause of an error or failure pattern, and produces a structured summary report.

Assumptions:
- Logs are plain text files on disk.
- The agent has access only to local tools (file system read, basic search, parsing).
- No external or paid services are used.

Step-by-step expected flow:
1. User submits a request pointing to a local log directory and a short problem description.
2. The agent creates a new Agent Run and persists initial state.
3. The planner decides which logs to inspect and in what order.
4. The agent reads log files using a file system tool.
5. The agent searches for errors, warnings, and anomalous patterns.
6. The agent summarizes findings and hypothesizes a root cause.
7. The output is validated for structure, completeness, and consistency.
8. If validation fails, the agent attempts repair (re-plan, re-read logs, or revise summary).
9. The agent produces a final structured report and completes the run.

Final Output Format (high level):
- Error summary
- Suspected root cause
- Supporting log evidence
- Confidence level
- Suggested next steps

## 7) Definition of Done for v1

Version 1 is considered DONE when all of the following are true:

Functional:
- The system can execute the Golden Path log analysis agent end-to-end.
- A user can submit a request pointing to a local log directory.
- The agent reads logs, analyzes them, and produces a structured report.

Reliability & Observability:
- Every agent run is persisted to storage.
- Every agent step (plan, tool call, validation) is recorded.
- Every tool call request and response is logged.
- Agent runs can be replayed from persisted state without re-running tools.

Failure Handling:
- At least one failure scenario can be intentionally triggered (e.g., invalid output format or missing evidence).
- The failure is detected explicitly (not silently ignored).
- The agent attempts at least one repair strategy and continues execution.

Constraints:
- No paid APIs or cloud services are used.
- The system runs fully on a local machine.

Out of Scope for v1:
- High accuracy or “smart” reasoning.
- Advanced ML-based failure classification.
- Performance optimization.

## 8) Core Components & Responsibilities

This section defines the core components of the system and their responsibilities. These are contracts, not implementations. Each component has a single, clear purpose.

### Agent Orchestrator
Responsibility:
- Entry point for executing agent runs.
- Creates new agent runs and resumes existing ones.
- Controls the main execution loop (plan → act → validate).
- Enforces limits such as max steps and timeouts.

Does NOT:
- Build prompts.
- Call tools directly.
- Perform validation or repair logic.

---

### Agent State Machine
Responsibility:
- Represents the durable state of an agent run.
- Tracks current step, memory, tool history, and status.
- Provides transitions between steps (running, failed, completed).

Does NOT:
- Call external systems.
- Make decisions about what to do next.

---

### Planner
Responsibility:
- Decides the next action for the agent based on current state.
- Builds prompts for the LLM.
- Parses LLM responses into structured actions (tool call, summary, stop).

Does NOT:
- Execute tools.
- Persist state.
- Detect failures.

---

### LLM Client
Responsibility:
- Sends prompts to a local LLM.
- Returns raw LLM responses.
- Abstracts away the underlying LLM implementation.

Does NOT:
- Contain business logic.
- Interpret responses.

---

### Tool Executor
Responsibility:
- Executes tool calls requested by the planner.
- Provides controlled access to local resources (file system, search).
- Enforces tool-specific timeouts and error handling.

Does NOT:
- Decide which tool to call.
- Modify agent state directly.

---

### Output Validator
Responsibility:
- Validates agent outputs against expected structure and rules.
- Ensures required fields are present (evidence, summary, etc.).
- Flags invalid or incomplete outputs explicitly.

Does NOT:
- Attempt to fix failures.
- Re-run the planner.

---

### Failure Detector
Responsibility:
- Inspects agent state and validation results.
- Classifies failures (e.g., invalid output, missing evidence).
- Decides whether a repair attempt is required.

Does NOT:
- Modify prompts or agent state directly.

---

### Repair Engine
Responsibility:
- Applies a repair strategy when a failure is detected.
- Examples: rewrite prompt, truncate memory, retry step.
- Returns control back to the orchestrator.

Does NOT:
- Detect failures.
- Execute tools.

---

### Storage Layer
Responsibility:
- Persist agent runs, steps, and tool results.
- Support replaying agent runs from stored state.

Does NOT:
- Contain orchestration or business logic.

## 9) Core Data Models

This section defines the core data models used by the system. These are conceptual models, not Go structs yet. They represent what must be persisted and replayable.

---

### AgentRun
Represents a single execution of an agent for a user request.

Fields (conceptual):
- RunID: Unique identifier for the agent run.
- Goal: The user’s requested task description.
- Status: One of (Created, Running, Failed, Completed).
- CurrentStepIndex: Index of the step currently being executed.
- CreatedAt: Timestamp when the run was created.
- CompletedAt: Timestamp when the run finished (if applicable).
- PromptVersion: Identifier for the prompt template used.
- ModelVersion: Identifier for the LLM model used.
- MaxSteps: Safety limit to prevent infinite loops.

Purpose:
- Anchor object for replay, debugging, and experimentation.
- Everything else references an AgentRun.

---

### AgentStep
Represents a single step within an agent run.

Fields (conceptual):
- StepID: Unique identifier for the step.
- RunID: Reference to the parent AgentRun.
- StepType: One of (Plan, ToolCall, Validation, Repair).
- Input: The input provided to this step.
- Output: The output produced by this step.
- Status: One of (Pending, Succeeded, Failed).
- StartedAt: Timestamp when the step started.
- FinishedAt: Timestamp when the step finished.

Purpose:
- Allow step-by-step replay and inspection.
- Provide fine-grained observability into agent behavior.

---

### ToolCall
Represents a single execution of a tool.

Fields (conceptual):
- ToolCallID: Unique identifier.
- RunID: Reference to the parent AgentRun.
- StepID: Reference to the AgentStep that triggered the tool.
- ToolName: Name of the tool invoked.
- ToolInput: Input parameters provided to the tool.
- ToolOutput: Raw output returned by the tool.
- Status: One of (Succeeded, Failed).
- ErrorMessage: Error details if the tool failed.
- ExecutedAt: Timestamp of execution.

Purpose:
- Ensure all external interactions are auditable.
- Provide grounding evidence for agent conclusions.

---

### ValidationResult
Represents the result of validating an agent output.

Fields (conceptual):
- ValidationID: Unique identifier.
- RunID: Reference to the parent AgentRun.
- StepID: Reference to the validated step.
- IsValid: Boolean indicating success or failure.
- FailureReason: Description of why validation failed (if applicable).
- DetectedAt: Timestamp of validation.

Purpose:
- Make failures explicit instead of implicit.
- Enable analysis of common failure modes.

---

### RepairAttempt
Represents an attempt to repair a detected failure.

Fields (conceptual):
- RepairID: Unique identifier.
- RunID: Reference to the parent AgentRun.
- FailedStepID: Reference to the step being repaired.
- StrategyName: Name of the repair strategy used.
- StrategyInput: Data provided to the repair strategy.
- Outcome: One of (Applied, Skipped, Failed).
- AttemptedAt: Timestamp of attempt.

Purpose:
- Track which repair strategies work.
- Support later experimentation and ML training.

## 10) Failure Modes & Detection Signals

This section defines known failure modes for the Golden Path agent and the signals used to detect them. These are explicit, testable conditions — not subjective judgments.

---

### Failure Mode 1: Invalid Output Structure

Description:
The agent produces a final response that does not match the expected structured format.

Examples:
- Missing required sections (e.g., no root cause).
- Output is unstructured free text.
- Fields are present but empty.

Detection Signals:
- Schema validation failure.
- Required fields not found or empty.

Expected Handling:
- Trigger repair using prompt clarification.
- Retry the planning step with stricter instructions.

---

### Failure Mode 2: Missing Supporting Evidence

Description:
The agent claims a root cause but does not reference specific log lines or files.

Examples:
- Conclusions without citations.
- Vague statements such as “an error occurred” without evidence.

Detection Signals:
- No tool call references associated with claims.
- Evidence section is empty or generic.

Expected Handling:
- Force re-analysis of logs.
- Require explicit evidence extraction before summarization.

---

### Failure Mode 3: Hallucinated Facts

Description:
The agent invents log entries, file names, or errors that do not exist in the provided data.

Examples:
- Referencing a log file that was never read.
- Mentioning error codes not present in any tool output.

Detection Signals:
- Output references data not present in recorded ToolCalls.
- Mismatch between claimed evidence and tool outputs.

Expected Handling:
- Invalidate the step.
- Truncate memory and re-run analysis using only verified tool outputs.

---

### Failure Mode 4: Repetitive or Looping Behavior

Description:
The agent repeatedly performs the same action without making progress.

Examples:
- Re-reading the same log files multiple times.
- Issuing identical tool calls repeatedly.

Detection Signals:
- Identical tool calls across consecutive steps.
- No change in agent state or conclusions across steps.

Expected Handling:
- Enforce step limit.
- Apply repair strategy to change planning approach or stop execution.

---

### Failure Mode 5: Overconfident Conclusions

Description:
The agent produces a confident root cause despite weak or ambiguous evidence.

Examples:
- High-confidence language with minimal supporting logs.
- Ignoring uncertainty in data.

Detection Signals:
- Confidence level is high but evidence count is low.
- Contradictory log signals not acknowledged.

Expected Handling:
- Require uncertainty acknowledgment.
- Downgrade confidence level or request further analysis.

---

### Failure Mode 6: Tool Misuse

Description:
The agent selects inappropriate tools or uses tools incorrectly.

Examples:
- Searching logs without specifying error keywords.
- Attempting to summarize before reading any logs.

Detection Signals:
- Tool calls made out of logical order.
- Tool inputs missing required parameters.

Expected Handling:
- Reject the tool call.
- Re-plan with explicit tool usage guidance.

## 11) Evaluation Metrics & Experiment Plan

This section defines how we will measure accuracy, reliability, and hallucination reduction over time. Metrics must be computable from persisted runs (AgentRun, AgentStep, ToolCall, ValidationResult).

---

### Core Metrics (v1)

1) Structured Output Success Rate
Definition:
- Percentage of runs where the final output passes the Output Validator structure checks.

Why it matters:
- Ensures the agent produces consistently usable outputs.

---

2) Evidence Coverage Score
Definition:
- For each run, count the number of distinct evidence references (log file + line or snippet) included in the final report.
- Score can be normalized (e.g., 0 to 1) based on minimum evidence requirement.

Why it matters:
- Encourages grounded conclusions and reduces unsupported claims.

---

3) Hallucination Rate (Grounding Violations)
Definition:
- Percentage of runs where the output references facts not present in recorded ToolCalls.
- This is measured by matching referenced file names / error codes / snippets against tool outputs.

Why it matters:
- Direct measurement of hallucination-like behavior.

---

4) Repair Success Rate
Definition:
- Percentage of detected failures that are successfully repaired (run continues and completes with valid output).

Why it matters:
- Measures robustness and usefulness of repair strategies.

---

5) Average Steps per Successful Run
Definition:
- Average number of AgentSteps required to reach completion for successful runs.

Why it matters:
- Detects inefficiency, looping, and wasted work.

---

### Experiment Plan (initial)

Experiment A: Prompt Strictness
Question:
- Does a stricter output schema prompt reduce invalid output structure failures?

Method:
- Run the same input logs with PromptVersion A (baseline) and PromptVersion B (strict schema).
- Compare Structured Output Success Rate and Hallucination Rate.

---

Experiment B: Evidence-First Planning
Question:
- Does forcing the agent to extract evidence before summarizing reduce hallucinations?

Method:
- Add an intermediate step that collects evidence snippets before writing conclusions.
- Compare Evidence Coverage Score and Hallucination Rate.

---

Experiment C: Memory Truncation on Failure
Question:
- Does truncating memory after a hallucination signal reduce repeated hallucinations?

Method:
- When Failure Mode 3 triggers, apply truncation and retry.
- Compare Repair Success Rate and Hallucination Rate.

---

### Dataset for Repeatable Evaluation

We will create a small local evaluation set:
- 5 to 10 log folders (or log files) that represent different failure patterns.
- Each case includes a short “expected issue category” label (manual label is fine).

Purpose:
- Enables consistent comparisons across prompt/model/repair changes without guessing.

## 12) Prompt Contract & Output Schema

This section defines the contract between the Planner and the LLM. The LLM must produce outputs that conform to a strict structure so that validation, failure detection, and replay are possible.

---

### Prompt Contract (High-Level Rules)

The LLM is instructed that:
- It must only reason using information obtained from tool outputs.
- It must not invent file names, log entries, or error messages.
- Every conclusion must be supported by explicit evidence.
- If evidence is insufficient, it must state uncertainty instead of guessing.

The planner is responsible for enforcing this contract through prompt design.

---

### Required Output Structure (Conceptual Schema)

The final output of the agent must follow this structure exactly:

- error_summary:
  - Short description of the observed issue.
- suspected_root_cause:
  - Hypothesis explaining why the issue occurred.
- supporting_evidence:
  - List of evidence items.
  - Each item must reference:
    - log file name
    - log snippet or line reference
- confidence_level:
  - One of: Low, Medium, High
- suggested_next_steps:
  - Optional actions for further investigation or remediation.

---

### Structural Constraints

- All required fields must be present.
- supporting_evidence must contain at least one item.
- confidence_level must reflect the strength of evidence.
- The agent must not include information that cannot be traced to a ToolCall.

---

### Validation Expectations

The Output Validator will:
- Check presence of all required fields.
- Verify that supporting evidence references recorded ToolCalls.
- Reject outputs that are unstructured or partially filled.

If validation fails:
- The failure is explicit.
- A repair attempt may be triggered.

## 13) Agent Execution Lifecycle

This section defines the lifecycle of an agent run and the allowed state transitions. All execution must follow this lifecycle to ensure predictable behavior.

---

### AgentRun States

An AgentRun may be in exactly one of the following states at any time:

- Created
- Running
- Failed
- Completed

---

### State Descriptions

Created:
- The agent run has been initialized.
- No planning or tool execution has occurred yet.
- Initial state is persisted before execution begins.

Running:
- The agent is actively executing steps.
- The orchestrator controls progression through steps.
- The agent may enter this state multiple times during repair attempts.

Failed:
- The agent encountered a failure that could not be repaired.
- Execution stops permanently.
- Failure reason is persisted and observable.

Completed:
- The agent successfully produced a valid final output.
- No further steps are allowed.
- Final output is persisted.

---

### Allowed State Transitions

- Created → Running
- Running → Running (next step)
- Running → Running (after repair)
- Running → Completed
- Running → Failed

No other transitions are allowed.

---

### Step Execution Lifecycle

Each AgentStep follows this lifecycle:

1. Step is created with status Pending.
2. Step execution begins (status Running).
3. Step produces output (status Succeeded or Failed).
4. Output is validated.
5. Validation result is persisted.
6. If validation fails, Failure Detection may trigger repair.
7. Control returns to the orchestrator for the next decision.

---

### Execution Stop Conditions

Execution must stop immediately when:
- The agent reaches the Completed state.
- The agent reaches the Failed state.
- The maximum step limit is reached.
- A non-recoverable error occurs.

These conditions are enforced by the orchestrator.

## 14) Implementation Order & Build Plan (v1)

This section defines the strict order in which components will be implemented. We do not move to the next phase until the current phase works end-to-end.

---

### Phase 1: Skeleton & Persistence
Goal:
- Make agent runs persistable and replayable.

Implement:
- AgentRun data model
- AgentStep data model
- Storage layer (SQLite)
- Ability to create and load an AgentRun

Exit Criteria:
- Can create an AgentRun and store it.
- Can reload an AgentRun from storage and inspect its state.

---

### Phase 2: Orchestrator Loop (No LLM)
Goal:
- Prove the execution lifecycle without LLM dependency.

Implement:
- Orchestrator execution loop
- Step lifecycle management
- Max step enforcement
- Manual “fake” planner for testing

Exit Criteria:
- Orchestrator can execute a sequence of dummy steps.
- State transitions follow the defined lifecycle.

---

### Phase 3: Planner + Local LLM Integration
Goal:
- Introduce reasoning while keeping control.

Implement:
- LLM client (Ollama)
- Planner that builds prompts and parses structured output
- Prompt contract enforcement

Exit Criteria:
- Planner can produce a valid structured plan from LLM output.

---

### Phase 4: Tool Execution (File System)
Goal:
- Enable real interaction with logs.

Implement:
- File system tool
- Tool executor
- ToolCall persistence

Exit Criteria:
- Agent can read log files and record tool outputs.

---

### Phase 5: Validation & Failure Detection
Goal:
- Make failures explicit.

Implement:
- Output validator
- Failure detector for defined failure modes

Exit Criteria:
- Invalid outputs are detected and logged.

---

### Phase 6: Repair Strategies
Goal:
- Enable recovery from failure.

Implement:
- At least one repair strategy (e.g., prompt rewrite or retry)
- RepairAttempt persistence

Exit Criteria:
- Agent can recover from at least one failure scenario.

---

### Phase 7: Evaluation & Replay
Goal:
- Enable experimentation.

Implement:
- Replay mechanism
- Metric computation for defined evaluation metrics

Exit Criteria:
- Two runs can be compared using metrics.
