import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { HealthBadge, HealthDot } from '@/components/core/health-badge'

describe('HealthBadge', () => {
  it('renders healthy state', () => {
    render(<HealthBadge state="healthy" />)
    expect(screen.getByText('Healthy')).toBeInTheDocument()
  })

  it('renders critical state', () => {
    render(<HealthBadge state="critical" />)
    expect(screen.getByText('Critical')).toBeInTheDocument()
  })

  it('renders with icon when withIcon is true', () => {
    const { container } = render(<HealthBadge state="healthy" withIcon />)
    expect(container.querySelector('svg')).toBeInTheDocument()
  })

  it('renders text variant', () => {
    render(<HealthBadge state="healthy" variant="text" />)
    expect(screen.getByText('Healthy')).toBeInTheDocument()
  })
})

describe('HealthDot', () => {
  it('renders a dot element', () => {
    const { container } = render(<HealthDot state="healthy" />)
    const dot = container.querySelector('span')
    expect(dot).toBeInTheDocument()
    expect(dot).toHaveClass('bg-success')
  })
})
