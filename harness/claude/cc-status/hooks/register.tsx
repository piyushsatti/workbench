import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import type { Counts, Diet, Line, MemoryFile, Status } from '../types'

// The 5h and 7d rate limits, drawn at the right end of the prompt footer (SessionMode), before any
// mode labels. Your status-line script must save the JSON Claude Code passes it, once per session, to
// $XDG_STATE_HOME/claude/statusline/<session id>.json (default ~/.local/state), with an added
// `local` object holding `five_hour_reset` and `seven_day_reset` already formatted in local time
// (the `Line` type). The status line keeps the model line.
// The turn number and the compactions so far go beside the permission mode (PromptHint), before the hint.
// The memory files the context loaded are listed under the first notice below the logo, and by /mem.
// /diet opens a side pane with what the context is spent on, largest first.

const status = atom({ plugin: 'cc-status', key: 'status' } as const, { memory: null } satisfies Status)
const diet = atom({ plugin: 'cc-status', key: 'diet' } as const, null as Diet)
const line = atom({ plugin: 'cc-status', key: 'line' } as const, null as Line)
const counts = atom({ plugin: 'cc-status', key: 'counts' } as const, null as Counts)
const DIET_PANE = 'cc-status-diet'

// Catppuccin Mocha.
// The longest the counts and the limits rows run ("turn 999 · 99 compactions", "[5h 100% → 00:20]
// [7d 100% → Tue]"); a shorter one is filled with spaces to this, so what follows it keeps its column.
const COUNTS_COLS = 25
const LIMITS_COLS = 33
const C = { red: '#f38ba8', peach: '#fab387', yellow: '#f9e2af', green: '#a6e3a1', teal: '#94e2d5', subtext1: '#bac2de', overlay0: '#6c7086' }

// The notice that carries the memory list: the first one drawn since this module loaded.
let memoryNotice: string | null = null
// The poll of the sidecar file; a reload starts a new one.
let stopPoll: (() => void) | null = null
// Where the context bar's breakdown goes for your status-line script, beside the sidecar's JSON; set by startPoll.
let ctxFile: string | null = null
let ctxWriteFailed = false

// What the status line's context bar shows, in tokens: the fixed load (system prompt, tool schemas,
// memory files, skills, agents), and the conversation split into the person's words (input), the
// model's (output) and tool calls with their results (tools). Free is the rest of the window, the
// auto-compact reserve included.
export type CtxParts = { fixed: number; input: number; output: number; tools: number }

// The breakdown's `Messages` row is one number, reconciled to the last API response; it is shared out
// by the characters of each kind in the messages (thinking is not in them, so it falls to the others).
// `deferred` rows sit outside the window and are left out.
export function ctxParts(
  categories: readonly { name: string; tokens: number; kind: string }[],
  messages: readonly { role: string; text: string; toolUses: readonly { input: unknown; text?: string }[] }[],
): CtxParts {
  let fixed = 0
  let conversation = 0
  for (const c of categories) {
    if (c.kind !== 'used') continue
    if (/^messages$/i.test(c.name)) conversation += c.tokens
    else fixed += c.tokens
  }
  let input = 0
  let output = 0
  let tools = 0
  for (const m of messages) {
    if (m.role === 'assistant') output += m.text.length
    else input += m.text.length
    for (const t of m.toolUses) tools += JSON.stringify(t.input ?? '').length + (t.text?.length ?? 0)
  }
  const chars = input + output + tools
  if (!chars) return { fixed, input: conversation, output: 0, tools: 0 }
  const part = (n: number) => Math.round((conversation * n) / chars)
  return { fixed, input: part(input), output: part(output), tools: part(tools) }
}

// `width` cells over the window, a letter per cell: f fixed, i input, o output, t tools, . free. A part
// with any tokens gets at least one cell, taken from the largest part.
export function ctxCells(p: CtxParts, window: number, width: number): string {
  // No window to measure against (0 before the first response): all free. Dividing by it would give
  // Infinity cells, which the trimming loop below never brings down.
  if (!(window > 0)) return '.'.repeat(width)
  const order = ['fixed', 'input', 'output', 'tools'] as const
  const n = order.map(k => (p[k] > 0 ? Math.max(1, Math.round((p[k] * width) / window)) : 0))
  while (n.reduce((a, b) => a + b, 0) > width) {
    const i = n.indexOf(Math.max(...n))
    n[i] = (n[i] ?? 1) - 1
  }
  const used = order.map((k, i) => k[0]!.repeat(n[i] ?? 0)).join('')
  return used + '.'.repeat(width - used.length)
}

const tilde = (path: string, home: string) => (home && path.startsWith(home + '/') ? '~' + path.slice(home.length) : path)
const kTok = (n: number) => (n >= 1000 ? `${(n / 1000).toFixed(1)}k` : `${n}`)

// One row per file: type, path, tokens; a total row when there is more than one.
export function memoryRows(files: MemoryFile[], home: string): string[] {
  const width = Math.max(...files.map(f => f.type.length))
  const rows = files.map(f => `${f.type.padEnd(width)}  ${tilde(f.path, home)}  ${kTok(f.tokens)}`)
  if (files.length > 1) rows.push(`${files.length} files, ${kTok(files.reduce((t, f) => t + f.tokens, 0))} tokens`)
  return rows
}

// green < 50, yellow < 75, peach < 90, red at 90 and over.
const limitColor = (pct: number) => (pct >= 90 ? C.red : pct >= 75 ? C.peach : pct >= 50 ? C.yellow : C.green)

export type Seg = { text: string; color: string; bold?: boolean }

// The limits, drawn at the right of the footer (SessionMode): [5h N% → HH:MM] [7d N% → Day]
export function limits(l: NonNullable<Line>): Seg[] {
  const s: Seg[] = []
  const dim = (text: string) => s.push({ text, color: C.overlay0 })
  const windows = [
    ['5h', l.rate_limits?.five_hour?.used_percentage, l.local?.five_hour_reset],
    ['7d', l.rate_limits?.seven_day?.used_percentage, l.local?.seven_day_reset],
  ] as const
  for (const [label, used, reset] of windows) {
    if (used === undefined) continue
    const p = Math.floor(used)
    const pct = `${p}%`
    dim(`${s.length ? ' ' : ''}[${label} `)
    s.push(p >= 90 ? { text: pct, color: C.red, bold: true } : { text: pct, color: limitColor(p) })
    if (reset) (dim(' → '), s.push({ text: reset, color: C.subtext1 }))
    dim(']')
  }
  return s
}

// Over the transcript JSONL: a turn is a prompt the person typed (text, or text with images), not a
// tool result, a slash command's echo (`<command-name>…`), a meta line or a compaction's summary.
// The transcript keeps every message across compactions, so the count runs from the session's start.
export const COUNT_JQ = `
def prompt_text:
  .message.content
  | if type == "string" then .
    elif type == "array" and (map(.type) | index("tool_result") | not) then (map(select(.type == "text") | .text) | first // "")
    else null end;
reduce inputs as $m ({turns: 0, compactions: 0};
  if $m.type == "system" and $m.subtype == "compact_boundary" then .compactions += 1
  elif $m.type == "user" and ($m.isMeta | not) and ($m.isCompactSummary | not)
       and (($m | prompt_text) | type == "string" and (startswith("<") | not)) then .turns += 1
  else . end)`

async function recount($: EngineInterface) {
  const path = (await read($, line))?.transcript_path
  if (!path) return
  try {
    const r = await $.process.run(['jq', '-n', '-c', COUNT_JQ, path], { timeoutMs: 5000 })
    if (r.exitCode === 0) await update($, counts, () => JSON.parse(r.stdout) as NonNullable<Counts>)
  } catch {
    // jq missing or the transcript unreadable: keep the last count
  }
}

async function refresh($: EngineInterface) {
  // 'summary' estimates locally and sends no token-count requests.
  const { context } = await $.session.usage({ breakdown: 'summary' })
  const b = context.breakdown
  const memory = b?.memoryFiles.map(f => ({ path: f.path, type: f.type, tokens: f.tokens })) ?? null
  await update($, status, () => ({ memory }))
  if (!b || !ctxFile) return
  try {
    const messages = await $.session.messages()
    const parts = ctxParts(b.categories, 'deny' in messages ? [] : messages)
    await $.fs.write(ctxFile, JSON.stringify({ window: b.maxTokens, ...parts, cells: ctxCells(parts, b.maxTokens, 20) }))
  } catch (err) {
    // Once per load: the status line keeps its percentage without the file.
    if (!ctxWriteFailed) $.ui.log(`cc-status: cannot write ${ctxFile}: ${String(err)}`)
    ctxWriteFailed = true
  }
}

// Reads this session's sidecar file; redraws only when it changed.
async function startPoll($: EngineInterface) {
  stopPoll?.()
  const home = String((await $.env.get('HOME')) ?? '')
  const state = String((await $.env.get('XDG_STATE_HOME')) ?? '') || `${home}/.local/state`
  const path = `${state}/claude/statusline/${await $.session.id()}.json`
  ctxFile = path.replace(/\.json$/, '.ctx.json')
  let last = ''
  const poll = async () => {
    let raw = ''
    try {
      raw = String(await $.fs.read(path))
    } catch {
      return // not written yet: the status line has not run for this session
    }
    if (raw === last) return
    last = raw
    try {
      const parsed = JSON.parse(raw) as Line
      await update($, line, () => parsed)
      if ((await read($, counts)) === null) await recount($)
    } catch {
      // caught mid-write: the next poll reads it whole
    }
  }
  await poll()
  const timer = $.clock.every(2000, () => void poll())
  stopPoll = () => timer.cancel()
}

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    await startPoll($)
    await refresh($)
    await $.command.register({ name: 'mem', description: 'cc-status: list the memory files this context loaded' })
    await $.command.register({ name: 'diet', description: 'cc-status: open the context-diet pane' })
    return next(e)
  })

  // The turn may have loaded a nested CLAUDE.md.
  on('turn.complete', async ($, e, next) => {
    await refresh($)
    await recount($)
    return next(e)
  })

  // The prompt is in the transcript once the turn has started.
  on('turn.start', async ($, e, next) => {
    const r = await next(e)
    await recount($)
    return r
  })

  // A compaction changes the context too: redraw the bar and the memory list now, not at the next turn.
  on('session.compact', async ($, e, next) => {
    const r = await next(e)
    if (!('skip' in r)) await refresh($)
    await recount($)
    return r
  })

  on('command.run', { command: 'mem' }, async $ => {
    await refresh($)
    const s = await read($, status)
    const home = String((await $.env.get('HOME')) ?? '')
    if (!s.memory?.length) return { text: 'No memory files in this context.' }
    return { text: memoryRows(s.memory, home).join('\n') }
  })

  on('command.run', { command: 'diet' }, async $ => {
    // 'summary' estimates locally and sends no token-count requests.
    const { context } = await $.session.usage({ breakdown: 'summary' })
    const b = context.breakdown
    const rows = (b?.categories ?? [])
      .filter(c => c.kind === 'used' && c.tokens > 0)
      .sort((x, y) => y.tokens - x.tokens)
      .map(c => ({ name: c.name, tokens: c.tokens }))
    await update($, diet, () => ({ rows, total: b?.totalTokens ?? 0 }))
    await $.ui.open({ id: DIET_PANE, title: 'context diet' })
    return { text: 'Opened the context-diet pane.' }
  })

  on('ui.render', { component: 'Pane', requestId: DIET_PANE }, async ($, e) => {
    const { Box, Text } = $.ui.resolve(e)
    const d = await read($, diet)
    if (d === null) return <Text dimColor>Run /diet to measure.</Text>
    return (
      <Box flexDirection="column" paddingX={1}>
        {d.rows.map(r => (
          <Text key={r.name}>{kTok(r.tokens).padStart(6)}  {r.name}</Text>
        ))}
        <Text dimColor>{kTok(d.total).padStart(6)}  total</Text>
      </Box>
    )
  })

  // turn N · M compactions, then what the hooks beneath (pm's entry) and the engine (its hint) draw.
  // Spaces after the counts, up to their longest ("turn 999 · 99 compactions"), keep what follows at
  // the same column as the counts grow.
  on('ui.render', { component: 'PromptHint' }, async ($, e, next) => {
    const n = await read($, counts)
    if (n === null) return next(e)
    const { Box, Text } = $.ui.resolve(e)
    const noun = n.compactions === 1 ? ' compaction' : ' compactions'
    const cols = `turn ${n.turns} · ${n.compactions}${noun}`.length
    return (
      <Box flexDirection="row">
        <Text wrap="truncate-end">
          <Text color={C.overlay0}>turn </Text>
          <Text color={C.subtext1}>{String(n.turns)}</Text>
          <Text color={C.overlay0}> · </Text>
          <Text color={n.compactions ? C.peach : C.subtext1}>{String(n.compactions)}</Text>
          <Text color={C.overlay0}>{noun}{`${' '.repeat(Math.max(0, COUNTS_COLS - cols))}   `}</Text>
        </Text>
        {await next(e)}
      </Box>
    )
  })

  // The limits go on the last row; what the hooks beneath and the engine draw (om's gauges, the mode
  // labels) goes above them.
  on('ui.render', { component: 'SessionMode' }, async ($, e, next) => {
    const l = await read($, line)
    const segs = l === null ? [] : limits(l)
    if (!segs.length) return next(e)
    const { Box, Text } = $.ui.resolve(e)
    // Right-aligned, so the spaces go on the left: the row's left edge stays put as the numbers change.
    const cols = segs.reduce((n, s) => n + s.text.length, 0)
    return (
      <Box flexDirection="column" alignItems="flex-end">
        {await next(e)}
        <Text wrap="truncate-end">
          {' '.repeat(Math.max(0, LIMITS_COLS - cols))}
          {segs.map((s, i) => (
            <Text key={String(i)} color={s.color} bold={s.bold}>{s.text}</Text>
          ))}
        </Text>
      </Box>
    )
  })

  // The notice is redrawn as the engine drew it (dim text, green command), with the memory rows under it.
  on('ui.render', { component: 'InfoNotice' }, async ($, e, next) => {
    memoryNotice ??= e.requestId
    const s = await read($, status)
    if (e.requestId !== memoryNotice || !s.memory?.length) return next(e)
    const { Box, Text } = $.ui.resolve(e)
    const home = String((await $.env.get('HOME')) ?? '')
    return (
      <Box flexDirection="column">
        <Text dimColor>
          {e.props.text}
          {e.props.command ? <Text color={C.green}> · {e.props.command}</Text> : ''}
        </Text>
        {memoryRows(s.memory, home).map((row, i) => (
          <Text key={String(i)} color={C.teal} wrap="truncate-end">{i === 0 ? 'memory  ' : '        '}{row}</Text>
        ))}
      </Box>
    )
  })
}
