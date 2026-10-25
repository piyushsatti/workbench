export type MemoryFile = { path: string; type: string; tokens: number }

export type Status = {
  cwd: string
  branch: string | null
  worktree: boolean
  memory: MemoryFile[] | null
}

export type Diet = { rows: { name: string; tokens: number }[]; total: number } | null

// The fields of the status-line JSON the footer draws, as your status-line script saves it per session.
export type Line = {
  transcript_path?: string
  rate_limits?: { five_hour?: { used_percentage?: number }; seven_day?: { used_percentage?: number } }
  local?: { five_hour_reset?: string | null; seven_day_reset?: string | null }
} | null

// Typed prompts and compactions so far, counted from the transcript.
export type Counts = { turns: number; compactions: number } | null

declare module 'claude-code' {
  interface PluginState {
    'cc-status': { status: Status; diet: Diet; line: Line; counts: Counts }
  }
}
