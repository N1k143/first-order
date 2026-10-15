"use client";

import { useRef } from "react";

export function LoginButton() {
  const dialog = useRef<HTMLDialogElement>(null);

  return (
    <>
      <button
        className="login-button"
        type="button"
        onClick={() => dialog.current?.showModal()}
      >
        Войти
      </button>
      <dialog
        className="login-dialog"
        ref={dialog}
        aria-labelledby="login-title"
      >
        <h2 id="login-title">Вход в аккаунт</h2>
        <p>
          Авторизация пока не подключена. Демонстрационный каталог доступен без
          входа.
        </p>
        <form method="dialog">
          <button className="button" autoFocus>
            Понятно
          </button>
        </form>
      </dialog>
    </>
  );
}
