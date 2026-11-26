'use client';
import { categories } from '@/data/demo';
import { useResource } from './resources';
import { loadCategories } from './category-data';

export function useCategories() {
  return useResource('/categories', categories, loadCategories);
}
