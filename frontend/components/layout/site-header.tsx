import { LoginButton } from "@/components/auth/login-button";
import { Brand } from "@/components/ui/brand";

export function SiteHeader() {
  return (
    <header className="header wrap">
      <Brand />
      <nav aria-label="Основная навигация">
        <a href="/#catalog">Компании</a>
        <a href="/#how">Как это работает</a>
        <a href="/#questions">Вопросы</a>
      </nav>
      <div className="header-actions">
        <LoginButton />
        <a className="button header-button" href="/#catalog">
          Открыть каталог
        </a>
      </div>
    </header>
  );
}
