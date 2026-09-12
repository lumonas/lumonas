import { describe, expect, it } from 'vitest'
import { formatBytes, formatUptime, formatPercent, formatDuration } from '@/lib/format'

describe('formatBytes', () => {
  it('formats null as dash', () => {
    expect(formatBytes(null)).toBe('—')
  })

  it('formats undefined as dash', () => {
    expect(formatBytes(undefined)).toBe('—')
  })

  it('formats bytes', () => {
    expect(formatBytes(500)).toBe('500 B')
  })

  it('formats kilobytes', () => {
    expect(formatBytes(1024)).toBe('1 KB')
  })

  it('formats megabytes', () => {
    expect(formatBytes(1048576)).toBe('1 MB')
  })

  it('formats gigabytes', () => {
    expect(formatBytes(1073741824)).toBe('1 GB')
  })

  it('formats terabytes', () => {
    expect(formatBytes(1099511627776)).toBe('1.0 TB')
  })
})

describe('formatUptime', () => {
  it('formats minutes', () => {
    expect(formatUptime(300)).toBe('5m')
  })

  it('formats hours and minutes', () => {
    expect(formatUptime(3660)).toBe('1h 1m')
  })

  it('formats days and hours', () => {
    expect(formatUptime(90000)).toBe('1d 1h')
  })
})

describe('formatPercent', () => {
  it('formats percent', () => {
    expect(formatPercent(50)).toBe('50%')
  })

  it('formats percent with decimals', () => {
    expect(formatPercent(50.5, 1)).toBe('50.5%')
  })
})

describe('formatDuration', () => {
  it('formats short duration', () => {
    const start = new Date(Date.now() - 30000).toISOString()
    expect(formatDuration(start)).toMatch(/\d+s/)
  })

  it('formats longer duration', () => {
    const start = new Date(Date.now() - 120000).toISOString()
    expect(formatDuration(start)).toMatch(/\d+m \d+s/)
  })
})
