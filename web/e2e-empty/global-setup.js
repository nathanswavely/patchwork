/**
 * Global setup for the empty-instance e2e suite.
 *
 * 1. Wipes data/e2e-empty/ so every run starts from a genuinely fresh,
 *    unseeded database (zero users, zero patches).
 * 2. Builds the backend and spawns it on this checkout's empty-suite port
 *    (see e2e/ports.js).
 * 3. Creates the instance's first user through the REAL production path:
 *    requests a magic link (no SMTP configured, so the backend prints the
 *    verify URL to its log), extracts the token from captured output,
 *    verifies it, and — because an unrecognized email has no account yet —
 *    completes signup by choosing a username (docs/adr/013: the username is
 *    picked by the person, never derived from the email). The backend
 *    bootstraps this first account as instance admin. The resulting session
 *    token is written to .session.json for the specs.
 *
 * The returned function is Playwright's global teardown — it stops the
 * backend.
 */
import { spawn, execSync } from 'child_process';
import { existsSync, mkdirSync, rmSync, writeFileSync } from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';
import { EMPTY_API_PORT, EMPTY_API_URL, REPO_ROOT } from '../e2e/ports.js';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = REPO_ROOT;
const dataDir = path.join(repoRoot, 'data', 'e2e-empty');
const API = EMPTY_API_URL;

const FIRST_USER_EMAIL = 'first@example.com';
const FIRST_USER_NAME = 'first';

/** Pull the session cookie out of a response, or null if it set none. */
function sessionFrom(res) {
  const cookies = res.headers.getSetCookie
    ? res.headers.getSetCookie().join('; ')
    : (res.headers.get('set-cookie') || '');
  return cookies.match(/patchwork_session=([^;]+)/)?.[1] || null;
}

async function waitFor(check, what, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const result = await Promise.resolve().then(check).catch(() => null);
    if (result) return result;
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error(`Timed out waiting for ${what}`);
}

export default async function globalSetup() {
  rmSync(dataDir, { recursive: true, force: true });
  mkdirSync(dataDir, { recursive: true });

  // The Go binary embeds web/dist (embed.FS); a fresh checkout doesn't have
  // it yet and the build fails without it. The suite itself uses vite, not
  // the embedded assets.
  if (!existsSync(path.join(repoRoot, 'web', 'dist', 'index.html'))) {
    console.log('web/dist missing — building SPA once for embed.FS...');
    execSync('npx vite build', { cwd: path.join(repoRoot, 'web'), stdio: 'pipe', timeout: 180000 });
  }

  // Build once and run the binary directly: killing a `go run` wrapper on
  // Windows would orphan the actual server process.
  const bin = path.join(dataDir, process.platform === 'win32' ? 'patchwork-e2e.exe' : 'patchwork-e2e');
  console.log('Building backend for empty-instance suite...');
  execSync(`go build -o "${bin}" ./cmd/patchwork`, { cwd: repoRoot, stdio: 'pipe', timeout: 180000 });

  let output = '';
  // PATCHWORK_PORT overrides the port in the yaml so this stack lands on the
  // port e2e/ports.js picked for this checkout — two worktrees running this
  // suite at once must not collide.
  console.log(`Empty-instance stack: backend :${EMPTY_API_PORT}`);
  const server = spawn(bin, ['-config', path.join('web', 'e2e-empty', 'patchwork.e2e-empty.yaml')], {
    cwd: repoRoot,
    env: { ...process.env, PATCHWORK_PORT: String(EMPTY_API_PORT) },
  });
  // Go's log package writes to stderr; capture both to be safe.
  server.stdout.on('data', (d) => { output += d.toString(); });
  server.stderr.on('data', (d) => { output += d.toString(); });
  const teardown = () => { server.kill(); };

  try {
    await waitFor(
      () => fetch(`${API}/api/v1/health`).then((r) => r.ok),
      'backend health endpoint', 30000,
    );

    // First-user signup via magic link. Without SMTP the backend logs the
    // verify URL instead of emailing it.
    const res = await fetch(`${API}/api/v1/auth/magic-link`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Patchwork-Request': 'true' },
      body: JSON.stringify({ email: FIRST_USER_EMAIL }),
    });
    if (!res.ok) throw new Error(`magic-link request failed: ${res.status}`);

    const token = await waitFor(
      () => output.match(/\/api\/v1\/auth\/verify\/([A-Za-z0-9]+)/)?.[1],
      'magic link in server log', 10000,
    );

    // Accept: application/json takes the API branch of the verify handler
    // (no redirect).
    const verify = await fetch(`${API}/api/v1/auth/verify/${token}`, {
      headers: { Accept: 'application/json' },
      redirect: 'manual',
    });
    if (!verify.ok) {
      throw new Error(`magic-link verify failed: ${verify.status}`);
    }

    // On an empty instance this email has no account, so verifying hands back
    // a signup token rather than a session — the person still has to choose a
    // username (docs/adr/013). Completing that is what creates the user, and
    // that response carries the session. An instance where the account
    // already exists logs straight in, so handle both.
    let session = sessionFrom(verify);
    if (!session) {
      const body = await verify.json();
      if (body.status !== 'username_required' || !body.signup_token) {
        throw new Error(`verify set no session and no signup token: ${JSON.stringify(body)}`);
      }
      const signup = await fetch(`${API}/api/v1/auth/signup`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'X-Patchwork-Request': 'true' },
        body: JSON.stringify({
          token: body.signup_token,
          username: FIRST_USER_NAME,
          display_name: 'First User',
        }),
      });
      if (!signup.ok) {
        throw new Error(`signup completion failed: ${signup.status} ${await signup.text()}`);
      }
      session = sessionFrom(signup);
      if (!session) {
        throw new Error(`no session cookie from signup completion (status ${signup.status})`);
      }
    }

    writeFileSync(path.join(__dirname, '.session.json'), JSON.stringify({ token: session }));
    console.log(`Empty instance ready: first user @${FIRST_USER_NAME} created via magic link.`);
  } catch (err) {
    teardown();
    console.error('Backend output:\n' + output);
    throw err;
  }

  return teardown;
}
