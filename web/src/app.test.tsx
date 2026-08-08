import {afterEach, describe, expect, it, vi} from 'vitest';
import {cleanup, render, screen} from '@testing-library/react';
import {MemoryRouter} from 'react-router-dom';

import {AppRoutes} from './app';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe('AppRoutes', () => {
  it('shows a connected backend status', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({status: 'ok'}), {
          status: 200,
          headers: {'Content-Type': 'application/json'},
        }),
      ),
    );

    render(
      <MemoryRouter initialEntries={['/']}>
        <AppRoutes />
      </MemoryRouter>,
    );

    expect(screen.getByRole('heading', {name: 'KeebHub'})).toBeInTheDocument();
    expect(await screen.findByText('Backend connected')).toBeInTheDocument();
  });

  it('shows a retryable status when the backend is unavailable', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network unavailable')));

    render(
      <MemoryRouter initialEntries={['/']}>
        <AppRoutes />
      </MemoryRouter>,
    );

    expect(await screen.findByText('Backend unavailable')).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Retry'})).toBeInTheDocument();
  });

  it('renders a not-found page for an unknown client route', () => {
    render(
      <MemoryRouter initialEntries={['/not-a-route']}>
        <AppRoutes />
      </MemoryRouter>,
    );

    expect(screen.getByRole('heading', {name: 'Page not found'})).toBeInTheDocument();
  });

  it('renders the Discord login entry page', () => {
    render(
      <MemoryRouter initialEntries={['/login?return_to=%2Flistings%2F1001']}>
        <AppRoutes />
      </MemoryRouter>,
    );

    expect(screen.getByRole('heading', {name: 'Sign in'})).toBeInTheDocument();
    expect(screen.getByRole('link', {name: 'Continue with Discord'})).toHaveAttribute(
      'href',
      '/auth/discord?return_to=%2Flistings%2F1001',
    );
  });

  it('shows a safe OAuth error and request ID', () => {
    render(
      <MemoryRouter
        initialEntries={[
          '/login?error=account_disabled&request_id=request-123&return_to=https%3A%2F%2Fevil.example',
        ]}
      >
        <AppRoutes />
      </MemoryRouter>,
    );

    expect(screen.getByRole('alert')).toHaveTextContent('This KeebHub account is disabled');
    expect(screen.getByText('Support request ID: request-123')).toBeInTheDocument();
    expect(screen.getByRole('link', {name: 'Continue with Discord'})).toHaveAttribute(
      'href',
      '/auth/discord',
    );
  });

  it('uses a generic message for an unknown OAuth error', () => {
    render(
      <MemoryRouter initialEntries={['/login?error=provider_secret&request_id=%3Cunsafe%3E']}>
        <AppRoutes />
      </MemoryRouter>,
    );

    expect(screen.getByRole('alert')).toHaveTextContent('KeebHub could not complete the login');
    expect(screen.queryByText(/Support request ID/)).not.toBeInTheDocument();
  });
});
