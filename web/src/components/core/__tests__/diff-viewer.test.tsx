import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { DiffViewer } from '@/components/core/diff-viewer'

describe('DiffViewer', () => {
  it('shows additions and deletions counts', () => {
    render(<DiffViewer before="line1" after="line1\nline2" />)
    expect(screen.getByText('+1')).toBeInTheDocument()
  })

  it('renders with custom titles', () => {
    render(<DiffViewer before="a" after="b" titleBefore="Before" titleAfter="After" />)
    expect(screen.getByText('Before → After')).toBeInTheDocument()
  })

  it('renders line content', () => {
    render(<DiffViewer before="hello" after="world" />)
    expect(screen.getByText('hello')).toBeInTheDocument()
    expect(screen.getByText('world')).toBeInTheDocument()
  })

  it('renders unchanged content', () => {
    render(<DiffViewer before="same" after="same" />)
    expect(screen.getByText('same')).toBeInTheDocument()
  })
})
