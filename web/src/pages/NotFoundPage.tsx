import {Link} from 'react-router-dom';

export function NotFoundPage() {
  return (
    <main className="shell shell--centered">
      <section className="not-found">
        <p className="eyebrow">404</p>
        <h1>Page not found</h1>
        <p>The address does not match a KeebHub page.</p>
        <Link className="text-link" to="/">
          Return to health
        </Link>
      </section>
    </main>
  );
}
