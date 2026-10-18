import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function shipped(path) {
  return readFileSync(resolve(__dirname, path), 'utf8')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^[ \t]*\/\/.*$/gm, '');
}

const thread = shipped('../components/CommentThread.svelte');
const commentsGo = readFileSync(resolve(__dirname, '../../../internal/handler/comments.go'), 'utf8');

// #366: Edit and Delete were gated on `comment.is_mine`, a field no comment
// payload carries, so an author never saw Edit and only an admin saw Delete.
describe('a comment knows its author (#366)', () => {
  it('reads ownership off author_id, which the server sends', () => {
    expect(commentsGo).toContain('`json:"author_id"`');
    expect(thread).toContain("import { getUser } from '../stores/auth.svelte.js';");
    expect(thread).toContain('c.author_id === viewer.id');
  });

  it('never asks for a field the payload does not carry', () => {
    expect(commentsGo).not.toContain('is_mine');
    expect(thread).not.toContain('is_mine');
  });

  it('offers the author Edit and Delete on comments and replies alike', () => {
    expect(thread).toContain('{#if isMine(comment)}');
    expect(thread).toContain('{#if isMine(comment) || isAdmin}');
    expect(thread).toContain('{#if isMine(reply)}');
    expect(thread).toContain('{#if isMine(reply) || isAdmin}');
  });
});
