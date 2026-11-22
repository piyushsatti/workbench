import { expect, test } from 'claude-code/testing'

import { ctxCells, ctxParts, limits, memoryRows } from './register'

test('ctxParts: fixed rows summed; Messages shared out by input, output and tool characters', () => {
  const rows = [
    { name: 'System prompt', tokens: 6000, kind: 'used' },
    { name: 'System tools', tokens: 15000, kind: 'used' },
    { name: 'MCP tools', tokens: 5000, kind: 'used' },
    { name: 'MCP tools (deferred)', tokens: 9000, kind: 'deferred' },
    { name: 'Memory files', tokens: 8000, kind: 'used' },
    { name: 'Skills', tokens: 4000, kind: 'used' },
    { name: 'Messages', tokens: 86000, kind: 'used' },
    { name: 'Free space', tokens: 99000, kind: 'free' },
    { name: 'Autocompact buffer', tokens: 33000, kind: 'buffer' },
  ]
  const messages = [
    { role: 'user', text: 'u'.repeat(100), toolUses: [] },
    { role: 'assistant', text: 'a'.repeat(300), toolUses: [{ input: { c: 'x' }, text: 'r'.repeat(591) }] },
  ]
  expect(ctxParts(rows, messages)).toEqual({ fixed: 38000, input: 8600, output: 25800, tools: 51600 })
})

test('ctxCells: a cell per 5%, every part with tokens shown, free the rest', () => {
  expect(ctxCells({ fixed: 38000, input: 8600, output: 25800, tools: 51600 }, 256000, 20)).toBe('fffiootttt..........')
  expect(ctxCells({ fixed: 100, input: 0, output: 0, tools: 0 }, 256000, 20)).toBe('f...................')
  expect(ctxCells({ fixed: 300000, input: 1, output: 1, tools: 1 }, 256000, 20)).toBe('fffffffffffffffffiot')
})

test('ctxCells: no window measured yet is all free, and returns', () => {
  for (const window of [0, -1, Number.NaN]) {
    expect(ctxCells({ fixed: 100, input: 1, output: 0, tools: 0 }, window, 20)).toBe('.'.repeat(20))
  }
})

test('limits: 5h with its reset time, 7d with its reset day', () => {
  const l = {
    rate_limits: { five_hour: { used_percentage: 14 }, seven_day: { used_percentage: 79.5 } },
    local: { five_hour_reset: '00:20', seven_day_reset: 'Tue' },
  }
  expect(limits(l).map(s => s.text).join('')).toBe('[5h 14% → 00:20] [7d 79% → Tue]')
})

test('limits: nothing before the first response reports them', () => {
  expect(limits({})).toEqual([])
})

test('memory rows: type, path under ~, tokens, and a total', () => {
  const rows = memoryRows(
    [
      { path: '/home/user/.claude/CLAUDE.md', type: 'User', tokens: 1234 },
      { path: '/home/user/.claude/projects/x/memory/MEMORY.md', type: 'AutoMem', tokens: 300 },
    ],
    '/home/user',
  )
  expect(rows[0]).toBe('User     ~/.claude/CLAUDE.md  1.2k')
  expect(rows[1]).toBe('AutoMem  ~/.claude/projects/x/memory/MEMORY.md  300')
  expect(rows[2]).toBe('2 files, 1.5k tokens')
})

test('/diet measures the context and opens the pane', async ($, on) => {
  const opened: string[] = []
  on('session.usage', () => ({
    value: { rateLimits: [], context: { window: 1, tokens: 1, percent: 1, breakdown: { totalTokens: 30000, categories: [{ name: 'System tools', tokens: 12000, kind: 'used' }] } } },
  }) as never)
  on('ui.open', (_$, e) => {
    opened.push(JSON.stringify(e))
    return { value: undefined } as never
  })

  const ran = await $.command.run({ command: 'diet' } as never)

  expect(JSON.stringify(ran)).toContain('context-diet pane')
  expect(opened.join()).toContain('cc-status-diet')
})

test('/mem lists the memory files from the usage breakdown', async ($, on) => {
  on('env.get', () => ({ value: '/home/user' }) as never)
  on('session.usage', () => ({
    value: { rateLimits: [], context: { window: 1, tokens: 1, percent: 1, breakdown: { memoryFiles: [{ path: '/home/user/Source/CLAUDE.md', type: 'Project', tokens: 2400 }] } } },
  }) as never)

  const ran = await $.command.run({ command: 'mem' } as never)

  expect(JSON.stringify(ran)).toContain('Project  ~/Source/CLAUDE.md  2.4k')
})

test('a compaction re-measures the context, so the footer redraws without waiting for a turn', async ($, on) => {
  let measured = 0
  on('session.usage', () => {
    measured += 1
    return { value: { rateLimits: [], context: { window: 1, tokens: 1, percent: 1 } } } as never
  })
  const messages = [{ role: 'user', text: 'the summary', toolUses: [] }]
  on('session.compact', () => ({ messages }) as never)

  await $.session.compact({ trigger: 'manual', messages } as never)

  expect(measured).toBe(1)
})
