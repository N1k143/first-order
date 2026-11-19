import assert from 'node:assert/strict';
import test from 'node:test';
import { profileFields, portfolioFields } from '../src/lib/api/profile-fields';

test('profile mutation only contains editable database columns', () => {
  const form = new FormData();
  for (const [key, value] of Object.entries({
    name: ' Мария ',
    bio: '',
    skills: 'React, TypeScript',
    github: 'https://github.com/example',
    role: 'moderator',
    is_moderator: 'true',
    user_id: 'other-user',
    email: 'private@example.com',
    avatarUrl: 'data:image/png;base64,abc',
    updated_at: 'forged',
  }))
    form.set(key, value);
  assert.deepEqual(profileFields(form), {
    name: 'Мария',
    bio: null,
    avatar_url: null,
    skills: 'React, TypeScript',
    github_url: 'https://github.com/example',
  });
});

test('portfolio supports clearing nullable columns without changing ownership', () => {
  const form = new FormData();
  form.set('title', ' Проект ');
  form.set('description', '  ');
  form.set('url', '');
  form.set('developer_id', 'someone-else');
  assert.deepEqual(portfolioFields(form), {
    title: 'Проект',
    description: null,
    project_url: null,
  });
});
