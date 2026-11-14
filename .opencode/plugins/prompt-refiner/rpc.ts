import { Rpc } from "@opencode/plugin/rpc"

export interface RefineInput {
  rawPrompt: string
  previousDraft?: string
  feedback?: string
  questions?: string[]
  answers?: string
  allowQuestions?: boolean
  retryCount?: number
}

export interface RefineOutput {
  status: "ready" | "questions"
  refinedPrompt: string
  questions: string[]
  modelUsed: string
  attempts: string[]
}

export interface CreateSessionInput {
  title: string
}

export interface CreateSessionOutput {
  sessionID: string
}

export const PromptRefinerRpc = Rpc.define({
  id: "prompt-refiner",
  methods: {
    refine: {
      input: {
        type: "object",
        properties: {
          rawPrompt: { type: "string" },
          previousDraft: { type: "string" },
          feedback: { type: "string" },
          questions: {
            type: "array",
            items: { type: "string" },
          },
          answers: { type: "string" },
          allowQuestions: { type: "boolean" },
          retryCount: { type: "number" },
        },
        required: ["rawPrompt"],
        additionalProperties: false,
      },
      output: {
        type: "object",
        properties: {
          status: {
            type: "string",
            enum: ["ready", "questions"],
          },
          refinedPrompt: { type: "string" },
          questions: {
            type: "array",
            items: { type: "string" },
          },
          modelUsed: { type: "string" },
          attempts: {
            type: "array",
            items: { type: "string" },
          },
        },
        required: ["status", "refinedPrompt", "questions", "modelUsed", "attempts"],
        additionalProperties: false,
      },
      errors: {},
    },
    createSession: {
      input: {
        type: "object",
        properties: {
          title: { type: "string" },
        },
        required: ["title"],
        additionalProperties: false,
      },
      output: {
        type: "object",
        properties: {
          sessionID: { type: "string" },
        },
        required: ["sessionID"],
        additionalProperties: false,
      },
      errors: {},
    },
  },
  events: {},
})
