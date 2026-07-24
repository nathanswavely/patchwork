import { execSync } from 'child_process';
import { DB_PATH, REPO_ROOT } from './ports.js';

export default function globalSetup() {
  // The suite's own database (e2e/ports.js), not the dev instance's — a test
  // run must never wipe data someone was using. The backend has already
  // opened and migrated it by the time this runs: Playwright starts
  // webServer before globalSetup.
  console.log(`Seeding test database (${DB_PATH})...`);
  try {
    execSync(`go run ./cmd/seed/ -force -db ${DB_PATH}`, {
      cwd: REPO_ROOT, stdio: 'pipe', timeout: 30000,
    });
    console.log('Database seeded successfully.');
  } catch (e) {
    console.error('Failed to seed database:', e.stderr?.toString());
    throw e;
  }
}
