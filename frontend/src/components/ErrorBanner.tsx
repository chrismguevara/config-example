import { ApiError } from "../api/client";

export function ErrorBanner({ error }: { error: unknown }) {
  if (!error) return null;
  if (error instanceof ApiError) {
    return (
      <div className="banner error" role="alert">
        <strong>
          {error.status} {error.message}
        </strong>
        {error.isConflict && <div>Someone else changed these settings. The latest version has been loaded; please retry.</div>}
        {error.problems.length > 0 && (
          <ul>
            {error.problems.map((p, i) => (
              <li key={i}>
                <code>{p.path}</code> {p.message}
              </li>
            ))}
          </ul>
        )}
      </div>
    );
  }
  return (
    <div className="banner error" role="alert">
      {String(error)}
    </div>
  );
}
