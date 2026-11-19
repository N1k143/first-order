import type { ProfileUpdate, PortfolioItemUpdate } from '@/types/database';

import { optionalText } from './form-fields';

export function profileFields(form: FormData): ProfileUpdate {
  return {
    name: optionalText(form.get('name')) || '',
    avatar_url: optionalText(form.get('avatar_url')),
    bio: optionalText(form.get('bio')),
    skills: optionalText(form.get('skills')),
    github_url: optionalText(form.get('github')),
  };
}

export function portfolioFields(form: FormData): PortfolioItemUpdate {
  return {
    title: optionalText(form.get('title')) || '',
    description: optionalText(form.get('description')),
    project_url: optionalText(form.get('url')),
  };
}
