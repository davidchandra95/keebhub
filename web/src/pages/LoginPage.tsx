import {useSearchParams} from 'react-router-dom';

import {AppCard} from '../components/AppCard';

const errorMessages: Record<string, string> = {
  authorization_denied: 'Discord authorization was cancelled. You can try again when ready.',
  oauth_state_invalid: 'The login request expired or could not be verified. Please start again.',
  discord_unavailable: 'Discord could not complete the login. Please try again shortly.',
  account_disabled: 'This KeebHub account is disabled and cannot sign in.',
  authentication_unavailable: 'Discord login is not configured or is temporarily unavailable.',
  authentication_failed: 'KeebHub could not complete the login. Please try again.',
};

function safeReturnTo(value: string | null): string {
  if (
    value === null ||
    value.length > 2048 ||
    !value.startsWith('/') ||
    value.startsWith('//') ||
    value.includes('\\')
  ) {
    return '/';
  }
  return value;
}

function safeRequestID(value: string | null): string | null {
  if (value === null || !/^[a-zA-Z0-9_-]{1,128}$/.test(value)) {
    return null;
  }
  return value;
}

export function LoginPage() {
  const [searchParams] = useSearchParams();
  const errorCode = searchParams.get('error');
  const errorMessage =
    errorCode === null
      ? null
      : (errorMessages[errorCode] ?? 'KeebHub could not complete the login. Please try again.');
  const requestID = safeRequestID(searchParams.get('request_id'));
  const returnTo = safeReturnTo(searchParams.get('return_to'));
  const authParams = new URLSearchParams();
  if (returnTo !== '/') {
    authParams.set('return_to', returnTo);
  }
  const authURL = `/auth/discord${authParams.size === 0 ? '' : `?${authParams.toString()}`}`;

  return (
    <main className="shell shell--centered">
      <section className="login" aria-labelledby="login-title">
        <p className="eyebrow">Keyboard marketplace</p>
        <h1 id="login-title">Sign in</h1>
        <AppCard>
          <div className="login-copy" aria-live="polite">
            <h2>Continue with Discord</h2>
            {errorMessage === null ? (
              <p>KeebHub uses your basic Discord profile to create your marketplace identity.</p>
            ) : (
              <div className="login-error" role="alert">
                <strong>Login was not completed</strong>
                <p>{errorMessage}</p>
                {requestID !== null && <small>Support request ID: {requestID}</small>}
              </div>
            )}
            <a className="primary-link" href={authURL}>
              Continue with Discord
            </a>
          </div>
        </AppCard>
      </section>
    </main>
  );
}
