import { Plugin } from "@opencode/plugin/tui"
import { createSignal, Show } from "solid-js"
import { PromptRefinerRpc, type CreateSessionOutput, type RefineInput, type RefineOutput } from "./rpc"

type RefinerState =
  | { kind: "idle" }
  | { kind: "prompting" }
  | { kind: "loading"; label: string }
  | {
      kind: "questions"
      rawPrompt: string
      questions: string[]
      modelUsed: string
      isFallback: boolean
    }
  | {
      kind: "answering"
      rawPrompt: string
      questions: string[]
      modelUsed: string
      isFallback: boolean
    }
  | {
      kind: "preview"
      rawPrompt: string
      refinedPrompt: string
      modelUsed: string
      isFallback: boolean
      retryCount: number
      questions?: string[]
      answers?: string
    }
  | {
      kind: "revising"
      rawPrompt: string
      refinedPrompt: string
      modelUsed: string
      isFallback: boolean
      retryCount: number
      questions?: string[]
      answers?: string
    }
  | { kind: "error"; message: string }

function makeSessionTitle(rawPrompt: string): string {
  const singleLine = rawPrompt.replace(/\s+/g, " ").trim()
  return singleLine.length <= 60 ? singleLine : `${singleLine.slice(0, 57)}...`
}

function formatError(err: unknown): string {
  if (err instanceof Error && err.message) return err.message
  if (err && typeof err === "object") {
    const record = err as Record<string, unknown>
    if (typeof record.message === "string" && record.message) return record.message
    if (record.data && typeof record.data === "object") {
      const dataMsg = (record.data as Record<string, unknown>).message
      if (typeof dataMsg === "string" && dataMsg) return dataMsg
    }
    try {
      return JSON.stringify(err)
    } catch {
      return String(err)
    }
  }
  return String(err)
}

const ERROR_AUTODISMISS_MS = 12000

export default Plugin.define({
  id: "prompt-refiner.tui",
  setup(context) {
    const rpc = context.client.rpc(PromptRefinerRpc)
    const [uiState, setUiState] = createSignal<RefinerState>({ kind: "idle" })

    let activeRunId = 0
    let sending = false
    let errorTimer: ReturnType<typeof setTimeout> | undefined

    const clearErrorTimer = () => {
      if (errorTimer !== undefined) {
        clearTimeout(errorTimer)
        errorTimer = undefined
      }
    }

    const showError = (message: string) => {
      clearErrorTimer()
      setUiState({ kind: "error", message })
      errorTimer = setTimeout(() => {
        errorTimer = undefined
        setUiState((prev) => (prev.kind === "error" ? { kind: "idle" } : prev))
      }, ERROR_AUTODISMISS_MS)
    }

    const dismiss = () => {
      activeRunId++
      clearErrorTimer()
      setUiState({ kind: "idle" })
    }

    const executeRefine = async (params: RefineInput, loadingLabel: string) => {
      clearErrorTimer()
      const runId = ++activeRunId
      setUiState({ kind: "loading", label: loadingLabel })

      try {
        const location = context.location ?? context.data.location.default()
        const result = (await rpc.refine(params, { location })) as RefineOutput
        if (runId !== activeRunId) return

        const isFallback = result.attempts.length > 0
        if (result.status === "questions" && result.questions.length > 0 && params.allowQuestions) {
          setUiState({
            kind: "questions",
            rawPrompt: params.rawPrompt,
            questions: result.questions,
            modelUsed: result.modelUsed,
            isFallback,
          })
          return
        }

        setUiState({
          kind: "preview",
          rawPrompt: params.rawPrompt,
          refinedPrompt: result.refinedPrompt,
          modelUsed: result.modelUsed,
          isFallback,
          retryCount: params.retryCount ?? 0,
          questions: params.questions,
          answers: params.answers,
        })
      } catch (err) {
        if (runId !== activeRunId) return
        showError(formatError(err))
      }
    }

    const startNewRefinement = async (inlineInput?: string) => {
      if (sending) return
      const current = uiState()
      if (
        current.kind === "prompting" ||
        current.kind === "revising" ||
        current.kind === "answering"
      ) {
        return
      }

      let rawPrompt = inlineInput?.trim() ?? ""
      if (!rawPrompt) {
        if (current.kind === "loading") return
        clearErrorTimer()
        const promptRunId = ++activeRunId
        setUiState({ kind: "prompting" })

        const entered = await context.ui.dialog.prompt({
          title: "Refine Rough Prompt",
          placeholder: "Describe what you want done...",
        })
        if (promptRunId !== activeRunId) return

        rawPrompt = entered?.trim() ?? ""
        if (!rawPrompt) {
          setUiState({ kind: "idle" })
          return
        }
      }

      await executeRefine(
        {
          rawPrompt,
          allowQuestions: true,
          retryCount: 0,
        },
        "Refining prompt...",
      )
    }

    const answerQuestions = async () => {
      const snapshot = uiState()
      if (snapshot.kind !== "questions" || sending) return

      clearErrorTimer()
      const answerRunId = ++activeRunId
      setUiState({ ...snapshot, kind: "answering" })

      const entered = await context.ui.dialog.prompt({
        title: "Answer Clarifying Questions (leave empty to skip & generate)",
        placeholder: "Reply briefly (e.g. 1: ..., 2: ...) or press Enter to skip...",
      })
      if (answerRunId !== activeRunId) return

      if (entered === undefined || entered === null) {
        setUiState(snapshot)
        return
      }

      const trimmed = entered.trim()
      await executeRefine(
        {
          rawPrompt: snapshot.rawPrompt,
          questions: snapshot.questions,
          ...(trimmed ? { answers: trimmed } : {}),
          allowQuestions: false,
          retryCount: 0,
        },
        trimmed ? "Generating prompt from answers..." : "Generating prompt...",
      )
    }

    const skipQuestionsAndGenerate = async () => {
      const snapshot = uiState()
      if (snapshot.kind !== "questions" || sending) return

      await executeRefine(
        {
          rawPrompt: snapshot.rawPrompt,
          questions: snapshot.questions,
          allowQuestions: false,
          retryCount: 0,
        },
        "Generating prompt...",
      )
    }

    const revisePreview = async () => {
      const snapshot = uiState()
      if (snapshot.kind !== "preview" || sending) return

      clearErrorTimer()
      const reviseRunId = ++activeRunId
      setUiState({ ...snapshot, kind: "revising" })

      const entered = await context.ui.dialog.prompt({
        title: "Revise Draft",
        placeholder: "Tell refiner what to adjust (Esc to keep current draft)...",
      })
      if (reviseRunId !== activeRunId) return

      const feedback = entered?.trim() ?? ""
      if (!feedback) {
        setUiState(snapshot)
        return
      }

      await executeRefine(
        {
          rawPrompt: snapshot.rawPrompt,
          previousDraft: snapshot.refinedPrompt,
          feedback,
          ...(snapshot.questions ? { questions: snapshot.questions } : {}),
          ...(snapshot.answers ? { answers: snapshot.answers } : {}),
          allowQuestions: false,
          retryCount: snapshot.retryCount,
        },
        "Updating draft...",
      )
    }

    const retryPreview = async () => {
      const snapshot = uiState()
      if (snapshot.kind !== "preview" || sending) return

      const nextRetryCount = snapshot.retryCount + 1
      await executeRefine(
        {
          rawPrompt: snapshot.rawPrompt,
          previousDraft: snapshot.refinedPrompt,
          ...(snapshot.questions ? { questions: snapshot.questions } : {}),
          ...(snapshot.answers ? { answers: snapshot.answers } : {}),
          allowQuestions: false,
          retryCount: nextRetryCount,
        },
        `Regenerating prompt (attempt #${nextRetryCount + 1})...`,
      )
    }

    const acceptPreview = async () => {
      const snapshot = uiState()
      if (snapshot.kind !== "preview" || sending) return

      sending = true
      activeRunId++
      clearErrorTimer()
      setUiState({ kind: "idle" })

      try {
        const location = context.location ?? context.data.location.default()
        const route = context.ui.router.current()
        let sessionID: string

        if (route.type === "session") {
          sessionID = route.sessionID
        } else {
          const created = (await rpc.createSession(
            { title: makeSessionTitle(snapshot.rawPrompt) },
            { location },
          )) as CreateSessionOutput
          sessionID = created.sessionID
          context.ui.router.navigate({ type: "session", sessionID })
        }

        await context.client.session.prompt({
          sessionID,
          text: snapshot.refinedPrompt,
        })
      } catch (err) {
        showError(formatError(err))
      } finally {
        sending = false
      }
    }

    const isBannerVisible = () => {
      const kind = uiState().kind
      return kind !== "idle" && kind !== "prompting"
    }

    const bannerHeader = () => {
      const s = uiState()
      switch (s.kind) {
        case "loading":
          return `Prompt Refiner — ${s.label}`
        case "questions":
          return `Clarification needed via ${s.modelUsed}${s.isFallback ? " (fallback)" : ""}`
        case "answering":
          return `Clarification needed via ${s.modelUsed} — answering...`
        case "preview":
          return `Refined via ${s.modelUsed}${s.isFallback ? " (fallback)" : ""}`
        case "revising":
          return `Refined via ${s.modelUsed}${s.isFallback ? " (fallback)" : ""} — revising...`
        case "error":
          return "Prompt Refiner — Error"
        default:
          return ""
      }
    }

    const bannerBody = () => {
      const s = uiState()
      if (s.kind === "questions" || s.kind === "answering") {
        return s.questions.map((q, idx) => `${idx + 1}. ${q}`).join("\n")
      }
      if (s.kind === "preview" || s.kind === "revising") {
        return s.refinedPrompt
      }
      if (s.kind === "error") {
        return s.message
      }
      return ""
    }

    const bannerFooter = () => {
      const s = uiState()
      switch (s.kind) {
        case "loading":
          return "[Esc] Cancel"
        case "questions":
          return "[Enter] Answer  •  [Alt+T] Skip & Generate  •  [Esc] Cancel"
        case "preview":
          return "[Enter] Send  •  [Alt+R] Revise  •  [Alt+T] Retry  •  [Esc] Cancel"
        case "error":
          return "[Esc] Dismiss"
        default:
          return ""
      }
    }

    const RefinerBanner = () => (
      <box flexDirection="column" paddingLeft={1} paddingRight={1}>
        <text fg={context.theme.text.muted}>{bannerHeader()}</text>
        <Show when={bannerBody().length > 0}>
          <text fg={context.theme.text.base}>{bannerBody()}</text>
        </Show>
        <Show when={bannerFooter().length > 0}>
          <text fg={context.theme.text.muted}>{bannerFooter()}</text>
        </Show>
      </box>
    )

    const stopSessionSlot = context.ui.slot({
      append: "session.composer.top",
      render: () => (
        <Show when={isBannerVisible()}>
          <RefinerBanner />
        </Show>
      ),
    })

    const stopHomeSlot = context.ui.slot({
      prepend: "home.footer",
      render: () => (
        <Show when={context.ui.router.current().type === "home" && isBannerVisible()}>
          <RefinerBanner />
        </Show>
      ),
    })

    context.keymap.layer(() => {
      const kind = uiState().kind
      const canEnter = kind === "preview" || kind === "questions"
      const canRetryOrSkip = kind === "preview" || kind === "questions"
      const canDismiss =
        kind === "loading" || kind === "preview" || kind === "questions" || kind === "error"

      const bindings = ["prompt-refiner.refine"]
      if (canEnter) bindings.push("prompt-refiner.accept")
      if (canRetryOrSkip) bindings.push("prompt-refiner.retry")
      if (canDismiss) bindings.push("prompt-refiner.dismiss")

      return {
        mode: "global",
        priority: canEnter || canRetryOrSkip || canDismiss ? 100 : 10,
        commands: [
          {
            id: "prompt-refiner.refine",
            title: "Refine Prompt",
            group: "Prompt",
            bind: "alt+r",
            palette: true,
            slash: {
              name: "refine",
              arguments: true,
            },
            enabled: () => {
              const k = uiState().kind
              return k !== "prompting" && k !== "revising" && k !== "answering"
            },
            suggested: true,
            run: async (input) => {
              const inlineText = typeof input === "string" ? input.trim() : ""
              if (!inlineText && uiState().kind === "preview") {
                await revisePreview()
                return
              }
              await startNewRefinement(inlineText || undefined)
            },
          },
          {
            id: "prompt-refiner.accept",
            title: "Send or Answer Refined Prompt",
            group: "Prompt",
            bind: "return",
            palette: false,
            enabled: () => {
              const k = uiState().kind
              return k === "preview" || k === "questions"
            },
            run: async () => {
              const k = uiState().kind
              if (k === "preview") {
                await acceptPreview()
                return
              }
              if (k === "questions") {
                await answerQuestions()
                return
              }
              return false
            },
          },
          {
            id: "prompt-refiner.retry",
            title: "Retry or Skip Clarification",
            group: "Prompt",
            bind: "alt+t",
            palette: false,
            enabled: () => {
              const k = uiState().kind
              return k === "preview" || k === "questions"
            },
            run: async () => {
              const k = uiState().kind
              if (k === "preview") {
                await retryPreview()
                return
              }
              if (k === "questions") {
                await skipQuestionsAndGenerate()
                return
              }
              return false
            },
          },
          {
            id: "prompt-refiner.dismiss",
            title: "Cancel Refined Prompt",
            group: "Prompt",
            bind: "escape",
            palette: false,
            enabled: () => {
              const k = uiState().kind
              return (
                k === "loading" || k === "preview" || k === "questions" || k === "error"
              )
            },
            run: () => {
              const k = uiState().kind
              if (
                k !== "loading" &&
                k !== "preview" &&
                k !== "questions" &&
                k !== "error"
              ) {
                return false
              }
              dismiss()
            },
          },
        ],
        bindings,
      }
    })

    return () => {
      activeRunId++
      clearErrorTimer()
      stopSessionSlot()
      stopHomeSlot()
    }
  },
})
