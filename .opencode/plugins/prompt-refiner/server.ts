import { Plugin } from "@opencode/plugin"
import {
  PromptRefinerRpc,
  type CreateSessionInput,
  type CreateSessionOutput,
  type RefineInput,
  type RefineOutput,
} from "./rpc"

const FLASH_MODELS = [
  { providerID: "opencode", id: "deepseek-v4.1-flash-free" },
  { providerID: "google-vertex", id: "gemini-3.8-flash" },
] as const

const FALLBACK_SAFETY_MODEL = {
  providerID: "google-vertex",
  id: "gemini-4-hydrogen",
} as const

const MAX_VCS_ITEMS = 25

function getModelOrder(retryCount = 0) {
  const swapFlash = retryCount > 0 && retryCount % 2 === 1
  const orderedFlash = swapFlash
    ? [FLASH_MODELS[1], FLASH_MODELS[0]]
    : [FLASH_MODELS[0], FLASH_MODELS[1]]
  return [...orderedFlash, FALLBACK_SAFETY_MODEL]
}

function formatVcsItem(item: unknown): string | null {
  if (!item || typeof item !== "object") return null
  const record = item as Record<string, unknown>
  const rawPath =
    (typeof record.path === "string" && record.path) ||
    (typeof record.file === "string" && record.file) ||
    ""
  if (!rawPath) return null
  const normalized = rawPath.replaceAll("\\", "/")
  if (normalized.startsWith(".twdll/") || normalized.startsWith(".codegraph/")) {
    return null
  }
  const status =
    (typeof record.status === "string" && record.status) ||
    (typeof record.type === "string" && record.type) ||
    "modified"
  return `${status}: ${rawPath}`
}

async function getVcsSummary(ctx: { vcs: { status: () => Promise<unknown> } }): Promise<string> {
  try {
    const raw = await ctx.vcs.status()
    if (!Array.isArray(raw) || raw.length === 0) {
      return "(clean working tree)"
    }
    const lines: string[] = []
    for (const item of raw) {
      const formatted = formatVcsItem(item)
      if (formatted) lines.push(formatted)
      if (lines.length >= MAX_VCS_ITEMS) break
    }
    if (lines.length === 0) return "(clean working tree)"
    const suffix =
      raw.length > lines.length ? `\n... (${raw.length - lines.length} more entries omitted)` : ""
    return lines.join("\n") + suffix
  } catch {
    return "(vcs status unavailable)"
  }
}

function buildMetaPrompt(input: RefineInput, vcsSummary: string): string {
  const allowQuestions = input.allowQuestions === true
  const isRetry = (input.retryCount ?? 0) > 0 && !input.feedback

  const contextBlocks: string[] = []

  if (input.questions && input.questions.length > 0) {
    contextBlocks.push(
      "CLARIFYING QUESTIONS PREVIOUSLY ASKED:",
      input.questions.map((q, i) => `${i + 1}. ${q}`).join("\n"),
      "",
      "USER ANSWERS:",
      input.answers?.trim() ||
        "(Skipped by user — choose sensible defaults and note any key assumption under Requirements or Plan First.)",
    )
  }

  if (input.previousDraft && input.feedback) {
    contextBlocks.push(
      "PREVIOUS DRAFT:",
      input.previousDraft,
      "",
      "USER REVISION REQUEST (apply these adjustments to the draft):",
      input.feedback,
    )
  } else if (input.previousDraft && isRetry) {
    contextBlocks.push(
      "PREVIOUS DRAFT (regenerate a cleaner, tighter alternative phrasing without inventing details):",
      input.previousDraft,
    )
  }

  const clarifyRules = allowQuestions
    ? [
        "",
        "OPTIONAL CLARIFICATION RULE (USE SPARINGLY):",
        "- ONLY if the raw request has a genuine, blocking product/behavioral ambiguity that cannot be answered by inspecting the codebase (e.g. mutually exclusive behaviors or an unclear target when multiple distinct options exist), output ONLY:",
        "CLARIFY:",
        "1. <concise question 1>",
        "2. <concise question 2 (optional, max 3 questions total)>",
        "- Unknown file paths, symbol names, or build commands are NOT ambiguities — do NOT ask questions about them; instruct the agent to locate them instead.",
        "- If the goal is reasonably clear, skip CLARIFY and output the 5-section prompt directly.",
      ].join("\n")
    : "- Do NOT ask clarifying questions; output ONLY the 5-section Markdown prompt below."

  return [
    "Rewrite the user's rough request into a crisp, structured, agent-ready engineering prompt.",
    "",
    "STRICT ANTI-HALLUCINATION & OUTPUT RULES:",
    "- NEVER invent file paths, placeholder brackets like `[filepath]`, symbol/function names, CLI flags, external packages, or build/test commands (such as `npm run typecheck`) that are not explicitly present in RAW REQUEST, USER ANSWERS, USER REVISION REQUEST, or CURRENT REPO STATUS.",
    "- If exact target files are unknown, write `- Locate the relevant files for <area/feature> first.` under `### Context & Target Files`.",
    "- If the exact test/build command is not explicitly known from the request or target ecosystem, write `the project's relevant test/build checks` in Step 3 of `### Workflow / Next Steps`.",
    "- Preserve all explicit file paths, identifiers, constraints, and safety rules stated by the user verbatim.",
    "- Do NOT wrap the output in outer Markdown code fences or add conversational preamble/commentary.",
    "- Write in clear, direct English.",
    clarifyRules,
    "",
    "REQUIRED 5-SECTION TEMPLATE (when not outputting CLARIFY):",
    "### Objective",
    "<1-2 clear sentences stating the goal and definition of done>",
    "",
    "### Context & Target Files",
    "- <explicit files/patterns from request or CURRENT REPO STATUS, or \"Locate the relevant files for ... first.\">",
    "- <any known tech stack or environment reminders from context, if applicable>",
    "",
    "### Requirements",
    "1. <Specific requirement 1>",
    "2. <Specific requirement 2>",
    "3. <Edge cases if applicable>",
    "",
    "### Constraints (STRICT)",
    "- <explicit exclusions / DO NOT modify unrelated files / DO NOT install new external dependencies without asking first>",
    "- Keep changes minimal and scoped only to this task.",
    "",
    "### Workflow / Next Steps",
    "1. **Plan First:** Read the relevant files and reply with a brief step-by-step plan of what you intend to change. **Wait for my approval before writing any code.**",
    "2. **Execute:** Once approved, implement the changes.",
    "3. **Verify:** Run <known test/build command if specified or unambiguous, else \"the project's relevant test/build checks\"> to ensure there are no regressions.",
    "",
    "CURRENT REPO STATUS:",
    vcsSummary,
    "",
    "RAW REQUEST:",
    input.rawPrompt.trim(),
    ...(contextBlocks.length > 0 ? ["", ...contextBlocks] : []),
  ]
    .filter((line) => line !== undefined)
    .join("\n")
}

function cleanModelOutput(raw: string): string {
  let cleaned = raw.replace(/<think>[\s\S]*?<\/think>/gi, "").trim()
  const fenceMatch = cleaned.match(/^```(?:markdown|md|text)?\s*\r?\n([\s\S]*?)\r?\n```$/i)
  if (fenceMatch?.[1]) {
    cleaned = fenceMatch[1].trim()
  }
  return cleaned
}

function parseRefinerResponse(
  cleaned: string,
  allowQuestions: boolean,
): { status: "ready"; refinedPrompt: string; questions: string[] } | { status: "questions"; refinedPrompt: string; questions: string[] } | null {
  if (!cleaned || cleaned.length < 12) return null

  const clarifyMatch = cleaned.match(/^(?:\*\*|#+\s*)?CLARIFY(?:\*\*)?:\s*\r?\n([\s\S]+)$/i)
  if (clarifyMatch) {
    if (!allowQuestions) {
      return null
    }
    const questions = clarifyMatch[1]
      .split(/\r?\n/)
      .map((line) => line.replace(/^\s*(?:\d+[.)]|[-*•])\s*/, "").trim())
      .filter((line) => line.length > 0 && !line.startsWith("###"))
      .slice(0, 3)

    if (questions.length > 0) {
      return {
        status: "questions",
        refinedPrompt: "",
        questions,
      }
    }
    return null
  }

  if (cleaned.length < 32 || !cleaned.includes("### Objective")) {
    return null
  }

  return {
    status: "ready",
    refinedPrompt: cleaned,
    questions: [],
  }
}

function extractText(response: unknown): string {
  if (typeof response === "string") return response
  if (response && typeof response === "object") {
    const text = (response as Record<string, unknown>).text
    if (typeof text === "string") return text
  }
  return ""
}

function formatError(err: unknown): string {
  if (err instanceof Error && err.message) return err.message
  return String(err)
}

export default Plugin.define({
  id: "prompt-refiner",
  async setup(ctx) {
    await ctx.rpc.register(PromptRefinerRpc, {
      refine: async (rawInput): Promise<RefineOutput> => {
        const input = rawInput as RefineInput
        const rawPrompt = input.rawPrompt?.trim()
        if (!rawPrompt) {
          throw new Error("Prompt cannot be empty.")
        }

        const allowQuestions = input.allowQuestions === true
        const vcsSummary = await getVcsSummary(ctx)
        const prompt = buildMetaPrompt({ ...input, rawPrompt, allowQuestions }, vcsSummary)
        const models = getModelOrder(input.retryCount ?? 0)
        const attempts: string[] = []

        for (const model of models) {
          const label = `${model.providerID}/${model.id}`
          try {
            const response = await ctx.experimental.generate.text({
              model: { providerID: model.providerID, id: model.id },
              prompt,
            })
            const cleaned = cleanModelOutput(extractText(response))
            const parsed = parseRefinerResponse(cleaned, allowQuestions)
            if (!parsed) {
              attempts.push(`${label}: malformed or unexpected output format`)
              continue
            }
            return {
              ...parsed,
              modelUsed: label,
              attempts,
            }
          } catch (err) {
            attempts.push(`${label}: ${formatError(err)}`)
          }
        }

        throw new Error(`All refiner models failed (${attempts.join("; ")})`)
      },

      createSession: async (rawInput): Promise<CreateSessionOutput> => {
        const input = rawInput as CreateSessionInput
        const title = input.title?.trim() || "Refined Task"
        const created = await ctx.session.create({ title })
        const sessionID =
          typeof created === "string"
            ? created
            : (created as { id?: string })?.id ?? ""
        if (!sessionID) {
          throw new Error("Failed to resolve created session ID.")
        }
        return { sessionID }
      },
    })
  },
})
