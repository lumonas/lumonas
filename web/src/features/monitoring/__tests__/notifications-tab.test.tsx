import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest'
import { NotificationsTab } from '@/features/monitoring/notifications-tab'

const server = setupServer(
	http.get('/api/v1/notification-channels', () => HttpResponse.json([])),
	http.get('/api/v1/notification-rules', () => HttpResponse.json([])),
	http.get('/api/v1/notification-deliveries', () => HttpResponse.json([])),
)

beforeAll(() => server.listen())
afterAll(() => server.close())
afterEach(() => server.resetHandlers())

function renderWithQuery() {
	const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
	return render(
		<QueryClientProvider client={queryClient}>
			<NotificationsTab />
		</QueryClientProvider>,
	)
}

describe('NotificationsTab', () => {
	it('exposes SMTP username and password fields', async () => {
		const user = userEvent.setup()
		renderWithQuery()
		await user.click(await screen.findByRole('button', { name: 'Add channel' }))
		await user.click(screen.getByRole('button', { name: 'smtp' }))

		await waitFor(() => {
			expect(screen.getByLabelText('SMTP target')).toBeInTheDocument()
			expect(screen.getByLabelText('SMTP username / sender')).toBeInTheDocument()
			expect(screen.getByLabelText('SMTP password')).toBeInTheDocument()
		})
	})
})
