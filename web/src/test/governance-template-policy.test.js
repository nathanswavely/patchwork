/**
 * The setup form seeds "Who can join" from the chosen governance template
 * (docs/adr/039), so the frontend carries a copy of each template's
 * membership_policy. The originals are Go string constants in
 * internal/governance/defaults.go, and a copy that drifts is worse than no
 * copy at all: the form would show one answer, the server would store
 * another, and nothing would fail.
 *
 * So this reads the Go constants and compares. Adding a template fails here
 * too — an unseeded template would leave the claimant's policy at whatever
 * the last one set, which is the silent inheritance this whole control
 * exists to end.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { TEMPLATE_MEMBERSHIP_POLICY } from '../lib/governanceTemplates.js';

const defaultsGo = readFileSync(
  resolve(process.cwd(), '..', 'internal', 'governance', 'defaults.go'),
  'utf8',
);

/**
 * The membership_policy inside `const rules<Name> = ` + "`{ ... }`".
 * Deliberately literal about the shape it expects: a rules block that stops
 * matching is a rules block that moved, and the test should say so rather
 * than quietly find nothing.
 */
function goTemplatePolicy(template) {
  const constName = 'rules' + template[0].toUpperCase() + template.slice(1);
  const block = defaultsGo.match(new RegExp(`const ${constName} = \`([\\s\\S]*?)\``));
  expect(block, `${constName} not found in defaults.go`).toBeTruthy();
  const policy = block[1].match(/"membership_policy":\s*"([a-z_]+)"/);
  expect(policy, `${constName} declares no membership_policy`).toBeTruthy();
  return policy[1];
}

describe('governance template membership policies', () => {
  it('covers exactly the templates Go ships', () => {
    const valid = defaultsGo.match(/var ValidTemplates = \[\]string\{([^}]*)\}/);
    expect(valid).toBeTruthy();
    const goTemplates = [...valid[1].matchAll(/"([a-z]+)"/g)].map((m) => m[1]);
    expect(goTemplates.length).toBeGreaterThan(0);
    expect(Object.keys(TEMPLATE_MEMBERSHIP_POLICY).sort()).toEqual([...goTemplates].sort());
  });

  it('matches each template rules file', () => {
    for (const [template, policy] of Object.entries(TEMPLATE_MEMBERSHIP_POLICY)) {
      expect(goTemplatePolicy(template), `${template} template`).toBe(policy);
    }
  });

  it('seeds a door that is closed by default', () => {
    // Minimal is the form's starting template and the typical claim: one
    // person running a listing. If that seed were ever 'open', the claim
    // path would be back where it started.
    expect(TEMPLATE_MEMBERSHIP_POLICY.minimal).toBe('invite_only');
  });
});
