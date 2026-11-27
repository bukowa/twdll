# Coordinator Prompt — Two-Stage Gemini Flash Audit

Paste this into the main agent to audit a run directory (replace `<RUN_DIR>`, e.g. `tools/ghidra-audit/runs/batches/batch_002` or `tools/ghidra-audit/runs/reverify/reverify_001`):

```text
You are coordinating a parallel Ghidra C++ pseudocode audit for `<RUN_DIR>`.

Do NOT read the `.c` chunk files into your own context. Instead:

1. Ensure Tier 0 pre-scan has run:
   `.twdll/uv/venv/bin/python tools/ghidra-audit/prescan.py --batch-dir <RUN_DIR>`
2. List `<RUN_DIR>/chunks/chunk_*.c` and compare against `<RUN_DIR>/findings/chunk_*.json` to find pending chunks.
3. Read `tools/ghidra-audit/PROMPT_WORKER.md` once so you know the exact worker specification.
4. Spawn **6 to 10 parallel Gemini Flash subagents** (`Model: "flash"`, **HIGH thinking level**; assign **1 chunk per worker**, 10-15 functions per chunk):
   - Give each worker:
     - The full instructions from `tools/ghidra-audit/PROMPT_WORKER.md`
     - Its exact input file: `<RUN_DIR>/chunks/chunk_NNN.c`
     - Its exact output file: `<RUN_DIR>/findings/chunk_NNN.json`
   - Require valid JSON with exact 1-to-1 `reviewed_addrs` and allowed enum values.
5. Wait for all workers to finish (re-run any chunk rejected by validation).
6. Run:
   `.twdll/uv/venv/bin/python tools/ghidra-audit/merge_findings.py --batch-dir <RUN_DIR>`
7. Read `<RUN_DIR>/SUMMARY.md` and report:
   - `RECALL_GATE` status (blind-slice recall check) and `EXIT_GATE` status
   - Total chunks & functions reviewed (broken down by Tier 0 skipped vs. Tier 1 LLM reviewed)
   - Systemic clusters (`>= 2` non-STL skeletons) and `HIGH` severity findings with suspect layer and verbatim snippets.
```
