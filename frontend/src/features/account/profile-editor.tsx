'use client';

import Link from 'next/link';
import { useRef, useState, type ChangeEvent } from 'react';
import { Avatar } from '@/components/ui/avatar';
import { Field, Textarea, Notice } from '@/components/ui/primitives';
import { useAppState, updateDemo } from '@/lib/state/store';
import { profileFields } from '@/lib/api/profile-fields';

export function ProfileEditor() {
  const { profile } = useAppState();
  const [avatar, setAvatar] = useState(profile.avatarUrl || '');
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');
  const [reading, setReading] = useState(false);
  const uploadVersion = useRef(0);

  async function upload(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;
    setError('');
    if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type) || file.size > 256 * 1024) {
      setError('Выберите PNG, JPEG или WebP размером до 256 КБ.');
      return;
    }
    const version = ++uploadVersion.current;
    setReading(true);
    try {
      const bitmap = await createImageBitmap(file);
      bitmap.close();
      const result = await new Promise<string>((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(String(reader.result));
        reader.onerror = () => reject(new Error('Не удалось прочитать файл.'));
        reader.readAsDataURL(file);
      });
      if (version === uploadVersion.current) setAvatar(result);
    } catch {
      if (version === uploadVersion.current)
        setError('Не удалось открыть изображение. Выберите другой файл.');
    } finally {
      if (version === uploadVersion.current) setReading(false);
    }
  }

  return (
    <form
      className="card stack"
      onSubmit={(event) => {
        event.preventDefault();
        const values = new FormData(event.currentTarget);
        const fields = profileFields(values);
        const name = fields.name;
        if (!name) {
          setError('Укажите имя, не состоящее из пробелов.');
          return;
        }
        updateDemo((state) => ({
          ...state,
          profile: {
            ...state.profile,
            name,
            avatarUrl: avatar,
            bio: fields.bio || '',
            skills: [
              ...new Set(
                (fields.skills || '')
                  .split(',')
                  .map((value) => value.trim())
                  .filter(Boolean),
              ),
            ],
            github: fields.github_url || '',
          },
          applications: state.applications.map((application) =>
            application.profileId === state.profile.id ? { ...application, name } : application,
          ),
        }));
        setError('');
        setMessage('Профиль сохранён в этом браузере.');
      }}
    >
      <h3>Фото профиля</h3>
      <div className="button-row">
        <Avatar name={profile.name} src={avatar} size={80} />
        <div className="stack">
          <Field
            label="Загрузить фото"
            type="file"
            accept="image/png,image/jpeg,image/webp"
            onChange={upload}
            disabled={reading}
          />
          <p className="caption muted">
            PNG, JPEG или WebP · до 256 КБ. Фото сохраняется только в демопрофиле.
          </p>
        </div>
      </div>
      <Field
        label="Или ссылка на фото"
        type="url"
        pattern="https?://.*"
        placeholder="https://example.com/photo.jpg"
        value={avatar.startsWith('data:') ? '' : avatar}
        disabled={reading}
        onChange={(event) => setAvatar(event.target.value)}
      />
      {avatar && (
        <button
          type="button"
          className="text-button align-start"
          disabled={reading}
          onClick={() => setAvatar('')}
        >
          Удалить фото
        </button>
      )}
      <hr />
      <h3>Основная информация</h3>
      <Field
        label="Имя и фамилия"
        name="name"
        defaultValue={profile.name}
        required
        maxLength={100}
        autoComplete="name"
      />
      <Textarea label="О себе" name="bio" defaultValue={profile.bio} maxLength={2000} />
      <Field
        label="Навыки через запятую"
        name="skills"
        defaultValue={profile.skills.join(', ')}
        maxLength={300}
      />
      <Field
        label="Ссылка на GitHub"
        name="github"
        type="url"
        pattern="https?://.*"
        defaultValue={profile.github}
        placeholder="https://github.com/username"
      />
      <p className="small muted">
        Тип аккаунта:{' '}
        {profile.role === 'company'
          ? 'Представитель компании'
          : profile.role === 'moderator'
            ? 'Модератор'
            : 'Разработчик'}
        .{' '}
        <Link className="text-link" href="/settings">
          Email и пароль — в настройках →
        </Link>
      </p>
      <div className="button-row">
        <button className="button" disabled={reading}>
          {reading ? 'Загрузка фото…' : 'Сохранить изменения'}
        </button>
        <Link className="button secondary" href="/profile/portfolio">
          Редактировать портфолио
        </Link>
        {profile.role === 'company' && (
          <Link className="button secondary" href="/workspace/profile">
            Профиль компании
          </Link>
        )}
      </div>
      {error && <Notice error>{error}</Notice>}
      {message && <Notice>{message}</Notice>}
    </form>
  );
}
